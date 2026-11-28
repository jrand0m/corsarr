package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/catalogue"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/modules"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// Modules is the module store the plugin admin drives (modules.Manager).
type Modules interface {
	List() []modules.Info
	Get(id string) (modules.Info, bool)
	Install(ctx context.Context, module, sidecar []byte, expectedSHA string) (modules.Info, error)
	Rollback(id string) (modules.Info, error)
	Uninstall(id string) (modules.Info, error)
	Action(ctx context.Context, id, action string, values map[string]any, http plugin.HTTPDoer) (abi.ActionResult, error)
}

// AutoUpdateJob is the daily plugin auto-update.
const AutoUpdateJob = "plugins.autoupdate"

// Change actions recorded in History.
const (
	ChangeInstalled   = "installed"
	ChangeUpdated     = "updated"
	ChangeRolledBack  = "rolledback"
	ChangeUninstalled = "uninstalled"
)

// PluginChange is the data of a plugin.changed event.
type PluginChange struct {
	Action   string `json:"action"`
	ID       string `json:"id"`
	Version  string `json:"version"`
	Previous string `json:"previous,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Source   string `json:"source"` // catalogue | file
}

// Plugin admin errors (the HTTP layer maps them to statuses).
var (
	ErrPluginsLocked       = errors.New("plugin module changes from the web UI and the API are turned off by plugins.uiInstall")
	ErrModulesUnavailable  = errors.New("plugin modules are not available in this process")
	ErrCatalogueChanged    = errors.New("the catalogue now offers a different version; review it again")
	ErrNotInCatalogue      = errors.New("the catalogue does not offer this plugin")
	ErrIncompatible        = errors.New("this corsarr cannot run the module")
	ErrNotInstalledModule  = errors.New("no such plugin module")
	ErrCatalogueUnreadable = errors.New("no verified catalogue is available")
)

// InstanceRef names a configured instance of a module.
type InstanceRef struct {
	Section string `json:"section"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// ModuleView is an installed module as the API shows it.
type ModuleView struct {
	modules.Info
	UsedBy        []InstanceRef `json:"usedBy"`
	Source        string        `json:"source"`
	ConfigSection string        `json:"configSection,omitempty"`
}

// PluginPolicy is the configured install policy.
type PluginPolicy struct {
	UIInstall    bool   `json:"uiInstall"`
	AutoUpdate   string `json:"autoUpdate"`
	CatalogueURL string `json:"catalogueUrl"`
	LockKey      string `json:"lockKey"`
}

// CatalogueView is the catalogue as the API shows it.
type CatalogueView struct {
	Source CatalogueSource   `json:"source"`
	Offers []catalogue.Offer `json:"offers"`
	Policy PluginPolicy      `json:"policy"`
}

// CatalogueSource says where the offers come from and how fresh they are.
type CatalogueSource struct {
	URL       string     `json:"url"`
	Release   string     `json:"release,omitempty"`
	Generated *time.Time `json:"generated,omitempty"`
	FetchedAt *time.Time `json:"fetchedAt,omitempty"`
	Stale     bool       `json:"stale"`
	Error     string     `json:"error,omitempty"`
}

// PluginAdminDeps wire the plugin admin.
type PluginAdminDeps struct {
	Modules Modules // nil when the module engine is off
	Config  func() *config.Config
	HTTP    catalogue.Doer
	Actions func(id string) plugin.HTTPDoer // the outbound client for a module's actions
	Bus     interface {
		Publish(domain.Event) (domain.Event, error)
	}
	Reload  func(pluginID string) // rebuilds the instances of a plugin
	Keys    []catalogue.Key
	Version string
	Log     *slog.Logger
}

// PluginAdmin lists, installs, updates, rolls back and removes plugin
// modules from the catalogue. Changes run one at a time.
type PluginAdmin struct {
	d     PluginAdminDeps
	fetch *catalogue.Fetcher
	mu    sync.Mutex // one writer of the installed modules
}

// NewPluginAdmin returns the admin.
func NewPluginAdmin(d PluginAdminDeps) *PluginAdmin {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	cfg := d.Config()
	return &PluginAdmin{d: d, fetch: &catalogue.Fetcher{Dir: filepath.Join(cfg.ModulesDir(), ".catalogue"), HTTP: d.HTTP,
		Keys: d.Keys, MaxModule: int64(cfg.Plugins.MaxModuleMiB) << 20}}
}

// Policy is the configured install policy.
func (p *PluginAdmin) Policy() PluginPolicy {
	c := p.d.Config().Plugins
	return PluginPolicy{UIInstall: c.UIInstall, AutoUpdate: c.AutoUpdate, CatalogueURL: p.catalogueURL(), LockKey: "plugins.uiInstall"}
}

func (p *PluginAdmin) catalogueURL() string {
	if u := p.d.Config().Plugins.CatalogueURL; u != "" {
		return u
	}
	return catalogue.DefaultURL(p.d.Version)
}

// List returns the installed modules with the instances using them.
func (p *PluginAdmin) List() ([]ModuleView, error) {
	if p.d.Modules == nil {
		return nil, ErrModulesUnavailable
	}
	sources := p.sources()
	out := []ModuleView{}
	for _, info := range p.d.Modules.List() {
		out = append(out, p.view(info, sources))
	}
	return out, nil
}

// Get returns one installed module.
func (p *PluginAdmin) Get(id string) (ModuleView, error) {
	if p.d.Modules == nil {
		return ModuleView{}, ErrModulesUnavailable
	}
	info, ok := p.d.Modules.Get(id)
	if !ok {
		return ModuleView{}, ErrNotInstalledModule
	}
	return p.view(info, p.sources()), nil
}

func (p *PluginAdmin) view(info modules.Info, sources map[string]installRecord) ModuleView {
	v := ModuleView{Info: info, UsedBy: usedBy(p.d.Config(), info.ID), Source: "file"}
	if r, ok := sources[info.ID]; ok && info.SHA256 != "" && r.Versions[info.Version] == info.SHA256 {
		v.Source = "catalogue"
	}
	if info.Kind != "" {
		v.ConfigSection = plugin.Kind(info.Kind).Section()
	}
	return v
}

// usedBy lists the configured instances of a plugin, by section and key.
func usedBy(cfg *config.Config, id string) []InstanceRef {
	out := []InstanceRef{}
	for section, insts := range cfg.Instances {
		for key, inst := range insts {
			if inst.Type == id {
				out = append(out, InstanceRef{Section: section, Key: string(key), Name: inst.Name, Enabled: inst.Enabled})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Section+"."+out[i].Key < out[j].Section+"."+out[j].Key
	})
	return out
}

// Catalogue returns the offers of the configured catalogue. A fetch failure
// with a cached copy shows the copy as stale; without one, the error.
func (p *PluginAdmin) Catalogue(ctx context.Context, refresh bool) CatalogueView {
	url := p.catalogueURL()
	view := CatalogueView{Source: CatalogueSource{URL: url}, Offers: []catalogue.Offer{}, Policy: p.Policy()}
	snap, err := p.fetch.Get(ctx, url, refresh)
	if err != nil {
		view.Source.Error = err.Error()
		return view
	}
	generated, fetched := snap.Index.Generated, snap.FetchedAt
	view.Source.Release, view.Source.Generated, view.Source.FetchedAt = snap.Index.Release, &generated, &fetched
	if snap.Stale {
		view.Source.Stale, view.Source.Error = true, snap.Err.Error()
	}
	view.Offers = catalogue.Offers(snap.Index, p.installed(), p.d.Version)
	return view
}

func (p *PluginAdmin) installed() map[string]catalogue.Installed {
	out := map[string]catalogue.Installed{}
	if p.d.Modules == nil {
		return out
	}
	for _, info := range p.d.Modules.List() {
		if info.Status != modules.StatusUninstalled {
			out[info.ID] = catalogue.Installed{Version: info.Version, Permissions: info.Manifest.Permissions, Present: true}
		}
	}
	return out
}

func (p *PluginAdmin) unlocked() error {
	if p.d.Modules == nil {
		return ErrModulesUnavailable
	}
	if !p.d.Config().Plugins.UIInstall {
		return ErrPluginsLocked
	}
	return nil
}

// Install installs (or updates to) exactly version of id from the
// catalogue, as the owner approved it.
func (p *PluginAdmin) Install(ctx context.Context, id, version string) (ModuleView, error) {
	if err := p.unlocked(); err != nil {
		return ModuleView{}, err
	}
	return p.installFromCatalogue(ctx, id, version, "user")
}

func (p *PluginAdmin) installFromCatalogue(ctx context.Context, id, version, actor string) (ModuleView, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	url := p.catalogueURL()
	snap, err := p.fetch.Get(ctx, url, false)
	if err != nil {
		return ModuleView{}, fmt.Errorf("%w: %v", ErrCatalogueUnreadable, err)
	}
	e, ok := snap.Index.Find(id)
	switch {
	case !ok:
		return ModuleView{}, ErrNotInCatalogue
	case version != "" && e.Version != version:
		return ModuleView{}, fmt.Errorf("%w (offered: %s)", ErrCatalogueChanged, e.Version)
	}
	if reason := catalogue.Incompatible(e, p.d.Version); reason != "" {
		return ModuleView{}, fmt.Errorf("%w: it %s", ErrIncompatible, reason)
	}
	before, had := p.d.Modules.Get(id)
	mod, err := catalogue.Download(ctx, p.d.HTTP, url, e)
	if err != nil {
		return ModuleView{}, err
	}
	info, err := p.d.Modules.Install(ctx, mod, nil, e.SHA256)
	if err != nil {
		return ModuleView{}, err
	}
	p.remember(info, url)
	action, previous := ChangeInstalled, ""
	if had && before.Status != modules.StatusUninstalled {
		action, previous = ChangeUpdated, before.Version
	}
	p.changed(actor, PluginChange{Action: action, ID: id, Version: info.Version, Previous: previous, SHA256: info.SHA256, Source: "catalogue"})
	return p.view(info, p.sources()), nil
}

// Update moves an installed module to exactly version from the catalogue.
func (p *PluginAdmin) Update(ctx context.Context, id, version string) (ModuleView, error) {
	if err := p.unlocked(); err != nil {
		return ModuleView{}, err
	}
	if info, ok := p.d.Modules.Get(id); !ok || info.Status == modules.StatusUninstalled {
		return ModuleView{}, ErrNotInstalledModule
	}
	return p.installFromCatalogue(ctx, id, version, "user")
}

// Rollback makes the kept previous version active again.
func (p *PluginAdmin) Rollback(id string) (ModuleView, error) {
	if err := p.unlocked(); err != nil {
		return ModuleView{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	before, ok := p.d.Modules.Get(id)
	if !ok || before.Status == modules.StatusUninstalled {
		return ModuleView{}, ErrNotInstalledModule
	}
	info, err := p.d.Modules.Rollback(id)
	if err != nil {
		return ModuleView{}, err
	}
	v := p.view(info, p.sources())
	p.changed("user", PluginChange{Action: ChangeRolledBack, ID: id, Version: info.Version, Previous: before.Version, SHA256: info.SHA256, Source: v.Source})
	return v, nil
}

// Uninstall removes a module; the instances using it stay configured and
// report that the module is not installed.
func (p *PluginAdmin) Uninstall(id string) ([]InstanceRef, error) {
	if err := p.unlocked(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	before, ok := p.d.Modules.Get(id)
	if !ok || before.Status == modules.StatusUninstalled {
		return nil, ErrNotInstalledModule
	}
	source := p.view(before, p.sources()).Source
	if _, err := p.d.Modules.Uninstall(id); err != nil {
		return nil, err
	}
	p.forget(id)
	p.changed("user", PluginChange{Action: ChangeUninstalled, ID: id, Version: before.Version, SHA256: before.SHA256, Source: source})
	return usedBy(p.d.Config(), id), nil
}

// changed rebuilds the plugin's instances and records the change.
func (p *PluginAdmin) changed(actor string, c PluginChange) {
	if p.d.Reload != nil {
		p.d.Reload(c.ID)
	}
	p.d.Log.Info("plugin module changed", "action", c.Action, "plugin", c.ID, "version", c.Version, "actor", actor)
	if p.d.Bus != nil {
		if _, err := p.d.Bus.Publish(domain.Event{Type: domain.EvPluginChanged, Actor: actor, Data: c}); err != nil {
			p.d.Log.Warn("plugin change not recorded", "err", err)
		}
	}
}

// ErrUnknownInstance refuses an action for an instance that is not
// configured for the module.
var ErrUnknownInstance = errors.New("no instance of this plugin with that name in the config")

// Action runs a settings action with the form's values; for a configured
// instance ("section.key") its saved settings (secrets included) are the
// base the posted values overlay.
func (p *PluginAdmin) Action(ctx context.Context, id, action, instance string, values map[string]any) (abi.ActionResult, error) {
	if p.d.Modules == nil {
		return abi.ActionResult{}, ErrModulesUnavailable
	}
	merged, err := p.actionValues(id, instance, values)
	if err != nil {
		return abi.ActionResult{}, err
	}
	var http plugin.HTTPDoer
	if p.d.Actions != nil {
		http = p.d.Actions(id)
	}
	return p.d.Modules.Action(ctx, id, action, merged, http)
}

func (p *PluginAdmin) actionValues(id, instance string, values map[string]any) (map[string]any, error) {
	out := map[string]any{}
	if instance != "" {
		section, key, _ := strings.Cut(instance, ".")
		inst, ok := p.d.Config().Instances[section][domain.ObjectKey(key)]
		if !ok || inst.Type != id {
			return nil, ErrUnknownInstance
		}
		for k, v := range inst.Settings {
			out[k] = v
		}
	}
	for k, v := range values {
		if s, isText := v.(string); v == nil || (isText && s == "" && instance != "") {
			continue // an untouched field keeps the saved value
		}
		out[k] = v
	}
	return out, nil
}

// AutoUpdate installs newer catalogue versions of modules installed from
// the catalogue when plugins.autoUpdate allows them and they ask for no new
// permissions.
func (p *PluginAdmin) AutoUpdate(ctx context.Context) error {
	policy := p.d.Config().Plugins.AutoUpdate
	if p.d.Modules == nil || policy == "" || policy == catalogue.AutoOff {
		return nil
	}
	snap, err := p.fetch.Get(ctx, p.catalogueURL(), true)
	if err != nil {
		return err
	}
	sources := p.sources()
	var errs []error
	for _, info := range p.d.Modules.List() {
		e, ok := snap.Index.Find(info.ID)
		if !ok || info.Status != modules.StatusActive || p.view(info, sources).Source != "catalogue" ||
			!catalogue.WithinPolicy(policy, info.Version, e.Version) {
			continue
		}
		if catalogue.PermissionDiff(info.Manifest.Permissions, e.Permissions).Grew() {
			p.d.Log.Info("plugin update waits for approval: it asks for new permissions", "plugin", info.ID, "version", e.Version)
			continue
		}
		if _, err := p.installFromCatalogue(ctx, info.ID, e.Version, "system"); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", info.ID, err))
		}
	}
	return errors.Join(errs...)
}

// ── catalogue installs on disk ───────────────────────────────────────────────

// installRecord remembers which versions of a module came from a catalogue
// (version → SHA-256), so a rolled-back version still counts as one.
type installRecord struct {
	Versions map[string]string `json:"versions"`
	URL      string            `json:"url"`
}

func (p *PluginAdmin) sourcesPath() string { return filepath.Join(p.fetch.Dir, "installed.json") }

func (p *PluginAdmin) sources() map[string]installRecord {
	out := map[string]installRecord{}
	b, err := os.ReadFile(p.sourcesPath())
	if err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func (p *PluginAdmin) writeSources(s map[string]installRecord) {
	b, _ := json.MarshalIndent(s, "", "  ")
	if err := os.MkdirAll(p.fetch.Dir, 0o755); err == nil {
		tmp := p.sourcesPath() + ".tmp"
		if err = os.WriteFile(tmp, b, 0o644); err == nil {
			err = os.Rename(tmp, p.sourcesPath())
		}
		if err != nil {
			p.d.Log.Warn("catalogue install record not written", "err", err)
		}
	}
}

func (p *PluginAdmin) remember(info modules.Info, url string) {
	s := p.sources()
	r := s[info.ID]
	if r.Versions == nil {
		r.Versions = map[string]string{}
	}
	r.Versions[info.Version], r.URL = info.SHA256, url
	s[info.ID] = r
	p.writeSources(s)
}

func (p *PluginAdmin) forget(id string) {
	s := p.sources()
	delete(s, id)
	p.writeSources(s)
}

// Provision installs the plugins.install entries that are missing (or at
// another version) from the catalogue, before the configuration is
// validated. It returns the changes made, for History once it is open, and
// one error per entry that could not be installed.
func (p *PluginAdmin) Provision(ctx context.Context, list []config.PluginInstall) ([]PluginChange, []error) {
	if p.d.Modules == nil || len(list) == 0 {
		return nil, nil
	}
	var (
		changes []PluginChange
		errs    []error
	)
	bus := p.d.Bus
	rec := &changeRecorder{}
	p.d.Bus = rec
	defer func() { p.d.Bus = bus }()
	for _, want := range list {
		if info, ok := p.d.Modules.Get(want.ID); ok && info.Status != modules.StatusUninstalled && (want.Version == "" || info.Version == want.Version) {
			continue
		}
		if _, err := p.installFromCatalogue(ctx, want.ID, want.Version, "system"); err != nil {
			errs = append(errs, fmt.Errorf("plugins.install %s: %w", want.ID, err))
		}
	}
	for _, ev := range rec.evs {
		changes = append(changes, ev.Data.(PluginChange))
	}
	return changes, errs
}

// changeRecorder keeps the events of a provisioning run.
type changeRecorder struct{ evs []domain.Event }

func (r *changeRecorder) Publish(ev domain.Event) (domain.Event, error) {
	r.evs = append(r.evs, ev)
	return ev, nil
}

// CachedOffers returns the offers of the last verified catalogue copy, by
// id, without network access (none when nothing is cached).
func (p *PluginAdmin) CachedOffers() map[string]catalogue.Offer {
	out := map[string]catalogue.Offer{}
	snap, ok := p.fetch.Cached(p.catalogueURL())
	if !ok {
		return out
	}
	for _, o := range catalogue.Offers(snap.Index, p.installed(), p.d.Version) {
		out[o.Entry.ID] = o
	}
	return out
}
