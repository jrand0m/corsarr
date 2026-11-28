// Package modules installs, loads and swaps plugin modules: a versioned
// on-disk store with an active pointer and one previous version for
// rollback, SHA-256 recorded at install and checked on every load, and
// registry integration so a module registers exactly like a built-in
// plugin. Replacing a module hot-swaps every running instance onto the new
// version while the old one drains; unloading drains and unregisters.
package modules

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/schema"
	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// Status of an installed module.
type Status string

const (
	StatusActive   Status = "active"   // registered and usable
	StatusFailed   Status = "failed"   // installed but refused at load (Error says why)
	StatusUnloaded Status = "unloaded" // installed, not active
)

// Info describes one installed module for lists and the API.
type Info struct {
	ID        string        `json:"id"`
	Version   string        `json:"version"`
	Previous  string        `json:"previous,omitempty"`
	SHA256    string        `json:"sha256"`
	Kind      string        `json:"kind"`
	Status    Status        `json:"status"`
	Error     string        `json:"error,omitempty"`
	Instances int           `json:"instances"`
	Ops       []string      `json:"ops,omitempty"`
	StartTime time.Duration `json:"startTime,omitempty"`
	Manifest  abi.Manifest  `json:"manifest"`
}

// Options configure a Manager.
type Options struct {
	Dir      string
	Engine   *wasm.Engine
	Registry *plugin.Registry
	// DisabledBuiltins are built-in ids the owner switched off so that a
	// module may take their id.
	DisabledBuiltins []string
	Log              *slog.Logger
}

// entry is one active module id.
type entry struct {
	info    Info
	man     abi.Manifest
	ver     Version
	mod     *wasm.Module // compiled lazily
	handles map[*wasm.Handle]wasm.Binding
	load    sync.Mutex // one compilation at a time per entry
}

// Manager is safe for concurrent use.
type Manager struct {
	o        Options
	store    Store
	builtins map[string]bool
	disabled map[string]plugin.Registration

	log     atomic.Pointer[slog.Logger]
	mu      sync.Mutex
	entries map[string]*entry
	failed  map[string]Info
}

// SetLog replaces the manager's logger (instances built later use it).
func (m *Manager) SetLog(l *slog.Logger) { m.log.Store(l) }

func (m *Manager) logger() *slog.Logger { return m.log.Load() }

// New returns a manager over o.Dir. Built-in ids are taken from the registry
// as it is now; disabled built-ins are unregistered.
func New(o Options) *Manager {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	m := &Manager{o: o, store: Store{Dir: o.Dir}, builtins: map[string]bool{}, disabled: map[string]plugin.Registration{},
		entries: map[string]*entry{}, failed: map[string]Info{}}
	m.SetLog(o.Log)
	for _, man := range o.Registry.List("") {
		if man.Delivery != "module" {
			m.builtins[man.ID] = true
		}
	}
	for _, id := range o.DisabledBuiltins {
		if reg, ok := o.Registry.Unregister(id); ok {
			m.disabled[id] = reg
		}
	}
	return m
}

// Store exposes the on-disk layout.
func (m *Manager) Store() Store { return m.store }

// ErrBuiltin refuses a module that takes the id of an enabled built-in.
var ErrBuiltin = errors.New("id belongs to a built-in plugin; add it to plugins.disabledBuiltins to replace it")

// ErrNotInstalled is returned for an unknown module id.
var ErrNotInstalled = errors.New("no such plugin module")

// Register reads every active module's manifest (checking its SHA-256) and
// registers it; modules compile on first use or in Warm. Failures are kept
// with their reason and do not stop the others.
func (m *Manager) Register() {
	for _, id := range m.store.IDs() {
		active, _ := m.store.Active(id)
		if active == "" {
			continue
		}
		if err := m.registerStored(id, active); err != nil {
			m.fail(id, active, err)
		}
	}
}

func (m *Manager) registerStored(id, version string) error {
	v, err := m.store.Read(id, version, wasm.SHA256)
	if err != nil {
		return err
	}
	raw, err := wasm.ManifestBytes(v.Module, v.Manifest)
	if err != nil {
		return err
	}
	man, err := wasm.ParseManifest(raw)
	if err != nil {
		return err
	}
	if man.ID != id || man.Version != version {
		return fmt.Errorf("stored as %s %s but the manifest says %s %s", id, version, man.ID, man.Version)
	}
	return m.activate(v, man, nil)
}

func (m *Manager) fail(id, version string, err error) {
	m.logger().Warn("plugin module not loaded", "plugin", id, "version", version, "err", err)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failed[id] = Info{ID: id, Version: version, Status: StatusFailed, Error: err.Error()}
}

// Install verifies a module (manifest, links, one start, capabilities),
// stores it as the active version (keeping the old one as previous) and
// hot-swaps running instances onto it. expectedSHA, when set, must match.
func (m *Manager) Install(ctx context.Context, module, sidecar []byte, expectedSHA string) (Info, error) {
	sum := wasm.SHA256(module)
	if expectedSHA != "" && expectedSHA != sum {
		return Info{}, fmt.Errorf("%w: expected %s, got %s", ErrChecksum, short(expectedSHA), short(sum))
	}
	mod, err := m.o.Engine.Load(ctx, module, sidecar)
	if err != nil {
		return Info{}, err
	}
	man := mod.Manifest
	if err := m.checkID(man.ID); err != nil {
		_ = mod.Close(ctx)
		return Info{}, err
	}
	raw, _ := wasm.ManifestBytes(module, sidecar)
	v := Version{ID: man.ID, Version: man.Version, SHA256: sum, Module: module, Manifest: raw}
	if err := m.store.Write(v); err != nil {
		_ = mod.Close(ctx)
		return Info{}, err
	}
	if err := m.store.Activate(man.ID, man.Version); err != nil {
		_ = mod.Close(ctx)
		return Info{}, err
	}
	if err := m.activate(v, man, mod); err != nil {
		return Info{}, err
	}
	info, _ := m.Get(man.ID)
	return info, nil
}

func (m *Manager) checkID(id string) error {
	if _, off := m.disabled[id]; m.builtins[id] && !off {
		return fmt.Errorf("%s: %w", id, ErrBuiltin)
	}
	return nil
}

// activate registers (or re-registers) id at version v and moves running
// instances onto it. mod may be nil to compile on first use.
func (m *Manager) activate(v Version, man abi.Manifest, mod *wasm.Module) error {
	if err := m.checkID(man.ID); err != nil {
		return err
	}
	spec, _ := wasm.Spec(wasm.ManifestKind(man))
	m.mu.Lock()
	old := m.entries[man.ID]
	e := &entry{man: man, ver: v, mod: mod, handles: map[*wasm.Handle]wasm.Binding{}}
	e.info = Info{ID: man.ID, Version: man.Version, SHA256: v.SHA256, Kind: wasm.ManifestKind(man), Status: StatusActive, Manifest: man}
	m.entries[man.ID] = e
	delete(m.failed, man.ID)
	m.mu.Unlock()
	reg := plugin.Registration{Manifest: registryManifest(man, spec, mod), New: m.factory(man.ID, spec)}
	if err := m.o.Registry.Replace(reg); err != nil {
		return err
	}
	if old != nil {
		m.moveInstances(old, e)
	}
	return nil
}

func registryManifest(man abi.Manifest, spec wasm.KindSpec, mod *wasm.Module) plugin.Manifest {
	pm := plugin.Manifest{ID: man.ID, Kind: spec.Kind, Version: man.Version, APIVersion: plugin.APIVersion, Name: man.Name,
		Delivery: "module", Settings: schema.PluginFields(man.Settings)}
	if mod != nil {
		pm.Capabilities = mod.Caps.Ops
	}
	return pm
}

// moveInstances rebinds every running instance of old onto next's module and
// drains old in the background.
func (m *Manager) moveInstances(old, next *entry) {
	m.mu.Lock()
	handles := old.handles
	old.handles = map[*wasm.Handle]wasm.Binding{}
	m.mu.Unlock()
	if len(handles) > 0 {
		mod, err := m.compiled(next.info.ID)
		for h, b := range handles {
			if err != nil {
				continue // calls keep failing on the drained pool; the instance reports it
			}
			m.rebind(next, mod, h, b)
		}
	}
	m.closeLater(old)
}

// rebind moves one running instance onto mod; an instance closed meanwhile
// keeps nothing.
func (m *Manager) rebind(e *entry, mod *wasm.Module, h *wasm.Handle, b wasm.Binding) {
	p, err := mod.NewPool(b)
	if err != nil {
		return
	}
	if _, ok := h.Swap(p); !ok {
		p.Drain(context.Background())
		return
	}
	m.track(e, h, b)
}

func (m *Manager) track(e *entry, h *wasm.Handle, b wasm.Binding) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.handles[h] = b
	h.SetOnClose(m.untrack)
}

func (m *Manager) untrack(h *wasm.Handle) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		delete(e.handles, h)
	}
}

// drainTimeout bounds how long a replaced or unloaded module waits for
// in-flight calls before they are interrupted.
const drainTimeout = 15 * time.Second

func (m *Manager) closeLater(e *entry) {
	if e.mod == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), drainTimeout)
		defer cancel()
		_ = e.mod.Close(ctx)
	}()
}

// compiled returns the entry's module, compiling it on first use.
func (m *Manager) compiled(id string) (*wasm.Module, error) {
	m.mu.Lock()
	e, ok := m.entries[id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%s: %w", id, ErrNotInstalled)
	}
	e.load.Lock()
	defer e.load.Unlock()
	m.mu.Lock()
	mod := e.mod
	m.mu.Unlock()
	if mod != nil {
		return mod, nil
	}
	mod, err := m.o.Engine.Load(context.Background(), e.ver.Module, e.ver.Manifest)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		e.info.Status, e.info.Error = StatusFailed, err.Error()
		return nil, err
	}
	e.mod = mod
	return mod, nil
}

// factory builds instances of a module id like any registry factory.
func (m *Manager) factory(id string, spec wasm.KindSpec) plugin.Factory {
	return func(inst plugin.Instance) (any, error) {
		mod, err := m.compiled(id)
		if err != nil {
			return nil, plugin.Wrap(plugin.ClassConfig, "plugin module "+id+" cannot start", err)
		}
		b := wasm.Binding{Key: string(inst.Key), Settings: inst.Settings, Log: m.logger()}
		if inst.Env != nil {
			b.HTTP = inst.Env.HTTP()
		}
		p, err := mod.NewPool(b)
		if err != nil {
			return nil, err
		}
		h := wasm.NewHandle(p)
		m.mu.Lock()
		e := m.entries[id]
		m.mu.Unlock()
		if e != nil {
			m.track(e, h, b)
		}
		return spec.Adapter(h), nil
	}
}

// Warm compiles every registered module now (the server does this in the
// background at start so the first instance does not wait).
func (m *Manager) Warm() {
	for _, info := range m.List() {
		if info.Status == StatusActive {
			if _, err := m.compiled(info.ID); err != nil {
				m.logger().Warn("plugin module failed to load", "plugin", info.ID, "err", err)
			}
		}
	}
}

// Unload deactivates a module: running instances drain and fail from then
// on, the id leaves the registry (a disabled built-in of the same id comes
// back), files stay for a later load or rollback.
func (m *Manager) Unload(id string) error {
	m.mu.Lock()
	e, ok := m.entries[id]
	delete(m.entries, id)
	delete(m.failed, id)
	m.mu.Unlock()
	if err := m.store.Deactivate(id); err != nil {
		return err
	}
	if !ok {
		if active, _ := m.store.Active(id); active == "" && len(m.store.versions(id)) == 0 {
			return fmt.Errorf("%s: %w", id, ErrNotInstalled)
		}
		return nil
	}
	m.o.Registry.Unregister(id)
	if reg, was := m.disabled[id]; was {
		_ = m.o.Registry.Replace(reg)
	}
	m.closeLater(e)
	return nil
}

// Rollback makes the previous version active again.
func (m *Manager) Rollback(id string) (Info, error) {
	version, err := m.store.Swap(id)
	if err != nil {
		return Info{}, err
	}
	if err := m.registerStored(id, version); err != nil {
		m.fail(id, version, err)
		return Info{}, err
	}
	info, _ := m.Get(id)
	return info, nil
}

// Remove unloads a module and deletes its files.
func (m *Manager) Remove(id string) error {
	if err := m.Unload(id); err != nil && !errors.Is(err, ErrNotInstalled) {
		return err
	}
	return m.store.Remove(id)
}

// List returns every installed module (active, failed or unloaded), by id.
func (m *Manager) List() []Info {
	seen := map[string]bool{}
	var out []Info
	m.mu.Lock()
	for id, e := range m.entries {
		out = append(out, m.infoLocked(e))
		seen[id] = true
	}
	for id, f := range m.failed {
		out = append(out, f)
		seen[id] = true
	}
	m.mu.Unlock()
	for _, id := range m.store.IDs() {
		if !seen[id] {
			out = append(out, m.storedInfo(id))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *Manager) infoLocked(e *entry) Info {
	info := e.info
	_, info.Previous = m.store.Active(info.ID)
	info.Instances = len(e.handles)
	if e.mod != nil {
		info.Ops, info.StartTime = e.mod.Caps.Ops, e.mod.StartTime
	}
	return info
}

// storedInfo describes a module that is installed but not active.
func (m *Manager) storedInfo(id string) Info {
	active, previous := m.store.Active(id)
	info := Info{ID: id, Version: active, Previous: previous, Status: StatusUnloaded}
	if info.Version == "" {
		info.Version = previous
		if vs := m.store.versions(id); info.Version == "" && len(vs) > 0 {
			info.Version = vs[0]
		}
	}
	if v, err := m.store.Read(id, info.Version, wasm.SHA256); err == nil {
		info.SHA256 = v.SHA256
		if raw, err := wasm.ManifestBytes(v.Module, v.Manifest); err == nil {
			info.Manifest, _ = wasm.ParseManifest(raw)
			info.Kind = wasm.ManifestKind(info.Manifest)
		}
	}
	return info
}

// Get returns one module's info.
func (m *Manager) Get(id string) (Info, bool) {
	for _, info := range m.List() {
		if info.ID == id {
			return info, true
		}
	}
	return Info{}, false
}

// Module returns the compiled active module of id (for settings actions and
// conformance runs).
func (m *Manager) Module(id string) (*wasm.Module, error) { return m.compiled(id) }

// Action runs a settings action of module id with the form's current values.
func (m *Manager) Action(ctx context.Context, id, action string, values map[string]any, http plugin.HTTPDoer) (abi.ActionResult, error) {
	mod, err := m.compiled(id)
	if err != nil {
		return abi.ActionResult{}, err
	}
	return mod.Action(ctx, wasm.Binding{Key: "action", Settings: values, HTTP: http, Log: m.logger()}, action)
}

// Close unloads every module from memory (files and pointers stay).
func (m *Manager) Close(ctx context.Context) {
	m.mu.Lock()
	entries := m.entries
	m.entries = map[string]*entry{}
	m.mu.Unlock()
	for _, e := range entries {
		if e.mod != nil {
			_ = e.mod.Close(ctx)
		}
	}
}
