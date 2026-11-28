package indexers

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/instances"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugins/declarative"
	"github.com/jrand0m/corsarr/internal/store"
)

// Repo persists UI-managed instances (store.Indexers implements it).
type Repo interface {
	List(ctx context.Context) ([]store.ManagedIndexer, error)
	Get(ctx context.Context, key domain.ObjectKey) (store.ManagedIndexer, bool, error)
	Save(ctx context.Context, m store.ManagedIndexer, now time.Time) error
	Delete(ctx context.Context, key domain.ObjectKey) (bool, error)
}

// Catalog is the definition catalogue (declarative.Catalog implements it).
type Catalog interface {
	Entries() []declarative.Entry
	Load(dir string) []declarative.Entry
	Revision() int
	Declarative(id string) bool
}

// Health records and reads integration health (health.Registry implements it).
type Health interface {
	Report(ref domain.IntegrationRef, o health.Observation) domain.IntegrationHealth
	Get(ref domain.IntegrationRef) (domain.IntegrationHealth, bool)
	Blocked(ref domain.IntegrationRef) bool
}

// QueryLog counts queries per indexer for query limits (store.Searches implements it).
type QueryLog interface {
	Queries(ctx context.Context, indexer domain.ObjectKey, since time.Time) (int, time.Time, error)
	RecordQuery(ctx context.Context, indexer domain.ObjectKey, at time.Time) error
}

// KV is a small persisted settings store (store.Settings implements it).
type KV interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string) error
}

// Deps wire the service.
type Deps struct {
	Repo     Repo
	Catalog  Catalog
	Plugins  *plugin.Registry
	UserDir  string // owner definitions
	Config   func() *config.Config
	Running  func() []instances.Running // running indexer instances
	HTTP     instances.HTTPFor
	Plain    plugin.HTTPDoer // the shared client without health attribution
	Health   Health
	Queries  QueryLog
	KV       KV
	Apply    func() // rebuilds the running instances after a change
	Now      func() time.Time
	Log      *slog.Logger
	BasePath func() string
}

// Service is the single writer of UI-managed indexer instances.
type Service struct {
	d       Deps
	mu      sync.Mutex // serialises writes
	snap    sync.Mutex
	managed []store.ManagedIndexer
	grabs   *grabCache
}

// New builds the service; call Load before the first Merge.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.BasePath == nil {
		d.BasePath = func() string { return "" }
	}
	return &Service{d: d, grabs: newGrabCache(2048)}
}

// Load reads the owner definitions and the managed instances.
func (s *Service) Load(ctx context.Context) error {
	s.d.Catalog.Load(s.d.UserDir)
	return s.refresh(ctx)
}

func (s *Service) refresh(ctx context.Context) error {
	list, err := s.d.Repo.List(ctx)
	if err != nil {
		return err
	}
	s.snap.Lock()
	s.managed = list
	s.snap.Unlock()
	return nil
}

// Merged is cfg plus the managed instances, as the instance manager should run them.
func (s *Service) Merged(cfg *config.Config) *config.Config {
	s.snap.Lock()
	managed := s.managed
	s.snap.Unlock()
	return Merge(cfg, managed, s.d.Catalog.Declarative, s.d.Catalog.Revision())
}

// Errors the API maps to statuses.
var (
	ErrManaged  = errors.New("this indexer is defined in the config file; change it there")
	ErrNotFound = errors.New("no such indexer")
	ErrExists   = errors.New("an indexer with this key already exists")
)

// InvalidError carries field problems or a failed connection test.
type InvalidError struct {
	Fields []FieldError
	Test   *TestResult
}

func (e *InvalidError) Error() string {
	if e.Test != nil {
		return "the indexer test failed: " + e.Test.Message
	}
	if len(e.Fields) > 0 {
		return e.Fields[0].Error()
	}
	return "invalid indexer"
}

// Input is a create or update request; nil fields keep their value on update.
type Input struct {
	Key      domain.ObjectKey `json:"key"`
	Plugin   string           `json:"plugin"`
	Name     *string          `json:"name"`
	Enabled  *bool            `json:"enabled"`
	Settings map[string]any   `json:"settings"`
}

// reservedKeys collide with API or endpoint paths.
var reservedKeys = map[domain.ObjectKey]bool{AllID: true, "test": true, "definitions": true, "torznab": true, "draft": true}

func (s *Service) inConfig(key domain.ObjectKey) bool {
	_, ok := s.d.Config().Instances[Section][key]
	return ok
}

// Create adds an indexer; an enabled one must pass its test first.
func (s *Service) Create(ctx context.Context, in Input) (Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := domain.ParseObjectKey(string(in.Key)); err != nil {
		return Row{}, &InvalidError{Fields: []FieldError{{"key", err.Error()}}}
	}
	if reservedKeys[in.Key] {
		return Row{}, &InvalidError{Fields: []FieldError{{"key", "\"" + string(in.Key) + "\" is reserved; choose another key"}}}
	}
	if _, ok, err := s.d.Repo.Get(ctx, in.Key); err != nil || ok || s.inConfig(in.Key) {
		return Row{}, errors.Join(ErrExists, err)
	}
	m := store.ManagedIndexer{Key: in.Key, PluginID: in.Plugin, Name: deref(in.Name, string(in.Key)), Enabled: deref(in.Enabled, true)}
	return s.store(ctx, m, in.Settings, nil)
}

// Update changes a UI-managed indexer.
func (s *Service) Update(ctx context.Context, key domain.ObjectKey, in Input) (Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inConfig(key) {
		return Row{}, ErrManaged
	}
	m, ok, err := s.d.Repo.Get(ctx, key)
	if err != nil || !ok {
		return Row{}, errors.Join(ErrNotFound, err)
	}
	m.Name, m.Enabled = deref(in.Name, m.Name), deref(in.Enabled, m.Enabled)
	settings := in.Settings
	if settings == nil {
		settings = m.Settings
	}
	return s.store(ctx, m, settings, m.Secrets)
}

func deref[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

func (s *Service) store(ctx context.Context, m store.ManagedIndexer, settings map[string]any, prevSecrets map[string]string) (Row, error) {
	reg, ok := s.d.Plugins.Get(m.PluginID)
	if !ok || reg.Manifest.Kind != plugin.KindIndexer {
		return Row{}, &InvalidError{Fields: []FieldError{{"plugin", "no indexer plugin " + m.PluginID + " in this build"}}}
	}
	vals, errs := Validate(reg.Manifest, settings, prevSecrets)
	if len(errs) > 0 {
		return Row{}, &InvalidError{Fields: errs}
	}
	m.Settings, m.Secrets = vals.Settings, vals.Secrets
	if m.Enabled {
		if res := s.test(ctx, Instance(m), true); !res.OK {
			return Row{}, &InvalidError{Test: &res}
		}
	}
	if err := s.d.Repo.Save(ctx, m, s.d.Now()); err != nil {
		return Row{}, err
	}
	if !m.Enabled {
		s.d.Health.Report(ref(m.Key), health.Observation{Disabled: true})
	}
	return s.changed(ctx, m.Key)
}

// Delete removes a UI-managed indexer.
func (s *Service) Delete(ctx context.Context, key domain.ObjectKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inConfig(key) {
		return ErrManaged
	}
	ok, err := s.d.Repo.Delete(ctx, key)
	if err != nil || !ok {
		return errors.Join(ErrNotFound, err)
	}
	_, err = s.changed(ctx, key)
	if errors.Is(err, ErrNotFound) {
		err = nil
	}
	return err
}

func (s *Service) changed(ctx context.Context, key domain.ObjectKey) (Row, error) {
	if err := s.refresh(ctx); err != nil {
		return Row{}, err
	}
	s.d.Apply()
	return s.Get(ctx, key)
}

// ReloadDefinitions re-reads the owner definitions and rebuilds declarative instances.
func (s *Service) ReloadDefinitions() []declarative.Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.d.Catalog.Load(s.d.UserDir)
	s.d.Apply()
	return entries
}

func ref(key domain.ObjectKey) domain.IntegrationRef { return instances.RefOf(plugin.KindIndexer, key) }

// Row is one indexer as the settings screen lists it.
type Row struct {
	Key        domain.ObjectKey          `json:"key"`
	Name       string                    `json:"name"`
	Plugin     string                    `json:"plugin"`
	PluginName string                    `json:"pluginName"`
	Delivery   string                    `json:"delivery"`
	Source     string                    `json:"source"`              // config | ui
	ManagedBy  *config.Managed           `json:"managedBy,omitempty"` // where the config sets it
	Enabled    bool                      `json:"enabled"`
	Running    bool                      `json:"running"`
	Priority   int                       `json:"priority"`
	Settings   map[string]any            `json:"settings"`
	SecretsSet []string                  `json:"secretsSet"`
	Health     *domain.IntegrationHealth `json:"health,omitempty"`
	Torznab    string                    `json:"torznab"`
}

// List returns every indexer from the config file and the UI, sorted by key.
func (s *Service) List(ctx context.Context) ([]Row, error) {
	cfg := s.Merged(s.d.Config())
	running := map[domain.ObjectKey]bool{}
	for _, r := range s.d.Running() {
		running[r.Key] = true
	}
	inConfig := s.d.Config().Instances[Section]
	var out []Row
	for key, inst := range cfg.Instances[Section] {
		_, fromCfg := inConfig[key]
		out = append(out, s.row(inst, fromCfg, running[key]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Get returns one indexer row.
func (s *Service) Get(ctx context.Context, key domain.ObjectKey) (Row, error) {
	rows, err := s.List(ctx)
	if err != nil {
		return Row{}, err
	}
	for _, r := range rows {
		if r.Key == key {
			return r, nil
		}
	}
	return Row{}, ErrNotFound
}

func (s *Service) row(inst config.Instance, fromConfig, running bool) Row {
	r := Row{Key: inst.Key, Name: firstNonEmpty(inst.Name, string(inst.Key)), Plugin: inst.Type, PluginName: inst.Type,
		Source: "ui", Enabled: inst.Enabled, Running: running, Priority: inst.Settings.Int("priority", 25),
		Settings: map[string]any{}, SecretsSet: []string{}, Torznab: s.d.BasePath() + "/torznab/" + string(inst.Key) + "/api"}
	if fromConfig {
		r.Source = "config"
		r.ManagedBy = config.ManagedAt(Section+"."+string(inst.Key), inst.Loc)
	}
	secret := map[string]bool{}
	if reg, ok := s.d.Plugins.Get(inst.Type); ok {
		r.PluginName, r.Delivery = reg.Manifest.Name, reg.Manifest.Delivery
		for _, f := range reg.Manifest.Settings {
			secret[f.Key] = f.IsSecret()
		}
	}
	for k, v := range inst.Settings {
		switch {
		case k == RevisionKey:
		case secret[k]:
			r.SecretsSet = append(r.SecretsSet, k)
		default:
			r.Settings[k] = v
		}
	}
	sort.Strings(r.SecretsSet)
	if h, ok := s.d.Health.Get(ref(inst.Key)); ok {
		r.Health = &h
	}
	return r
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
