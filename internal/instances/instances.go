// Package instances builds the running plugin instances from the config's
// instance sections: one factory call per enabled
// instance with the shared outbound client scoped to the instance's health
// ref, an atomic swap on config change, and self-tests on start and on a
// schedule (never for notifiers, whose test sends a real message).
package instances

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// HTTPFor hands out the shared client scoped to one integration (httpx.Client).
type HTTPFor interface {
	For(ref domain.IntegrationRef) plugin.HTTPDoer
}

// Reporter records integration health (health.Registry).
type Reporter interface {
	Report(ref domain.IntegrationRef, o health.Observation) domain.IntegrationHealth
}

// Running is one built plugin instance.
type Running struct {
	Key      domain.ObjectKey
	Name     string
	PluginID string
	Kind     plugin.Kind
	Settings plugin.Settings
	Impl     any
}

// Ref is the instance's IntegrationHealth ref.
func (r Running) Ref() domain.IntegrationRef { return RefOf(r.Kind, r.Key) }

// RefOf names the health ref of an instance of a kind.
func RefOf(k plugin.Kind, key domain.ObjectKey) domain.IntegrationRef {
	return domain.IntegrationRef{Type: string(k), Key: key}
}

// Named is a running instance viewed through one extension point.
type Named[T any] struct {
	Running
	Impl T
}

type set map[plugin.Kind][]Running

// Manager owns the running instances.
type Manager struct {
	reg  *plugin.Registry
	http HTTPFor
	rep  Reporter
	log  *slog.Logger
	cur  atomic.Pointer[set]
	// applyMu serialises Apply: one writer of the running set.
	applyMu sync.Mutex
}

// New returns an empty manager.
func New(reg *plugin.Registry, http HTTPFor, rep Reporter, log *slog.Logger) *Manager {
	m := &Manager{reg: reg, http: http, rep: rep, log: log}
	m.cur.Store(&set{})
	return m
}

var sectionKinds = map[string]plugin.Kind{}

func init() {
	for _, k := range []plugin.Kind{plugin.KindIndexer, plugin.KindDownloader, plugin.KindMetadata, plugin.KindMediaServer, plugin.KindNotifier} {
		sectionKinds[k.Section()] = k
	}
}

type env struct {
	http plugin.HTTPDoer
}

func (e env) HTTP() plugin.HTTPDoer { return e.http }
func (e env) Now() time.Time        { return time.Now() }

// Apply makes the running instances match cfg: an instance whose plugin,
// name and settings are unchanged keeps running untouched (its sessions and
// health stay); changed or new ones are built; removed or disabled ones are
// closed when they can be. Instances that fail to build report a config
// health failure and are left out. The new set is swapped in atomically.
func (m *Manager) Apply(cfg *config.Config) {
	m.applyMu.Lock()
	defer m.applyMu.Unlock()
	prev := *m.cur.Load()
	next := set{}
	kept := map[any]bool{}
	for section, insts := range cfg.Instances {
		kind, ok := sectionKinds[section]
		if !ok {
			continue
		}
		for _, inst := range insts {
			if r, ok := running(prev, kind, inst); ok {
				next[kind], kept[r.Impl] = append(next[kind], r), true
				continue
			}
			if r, ok := m.build(kind, inst); ok {
				next[kind] = append(next[kind], r)
			}
		}
	}
	for _, list := range next {
		sort.Slice(list, func(i, j int) bool { return list[i].Key < list[j].Key })
	}
	m.cur.Store(&next)
	m.closeDropped(prev, kept)
}

// running finds the previous instance when its config is unchanged.
func running(prev set, kind plugin.Kind, inst config.Instance) (Running, bool) {
	if !inst.Enabled {
		return Running{}, false
	}
	for _, r := range prev[kind] {
		if r.Key == inst.Key {
			return r, Same(r, inst)
		}
	}
	return Running{}, false
}

// Same reports whether a running instance was built from this config (pure).
func Same(r Running, inst config.Instance) bool {
	return r.PluginID == inst.Type && r.Name == inst.Name && reflect.DeepEqual(r.Settings, inst.Settings)
}

// closeDropped closes previous instances that are no longer running.
func (m *Manager) closeDropped(prev set, kept map[any]bool) {
	for _, list := range prev {
		for _, r := range list {
			if c, ok := r.Impl.(interface{ Close() error }); ok && !kept[r.Impl] {
				if err := c.Close(); err != nil {
					m.log.Warn("close plugin instance", "section", r.Kind.Section(), "key", r.Key, "err", err)
				}
			}
		}
	}
}

func (m *Manager) build(kind plugin.Kind, inst config.Instance) (Running, bool) {
	ref := RefOf(kind, inst.Key)
	if !inst.Enabled {
		m.rep.Report(ref, health.Observation{Disabled: true})
		return Running{}, false
	}
	impl, err := m.construct(kind, inst)
	if err != nil {
		m.rep.Report(ref, health.Observation{Class: classOf(err, health.ClassConfig), Message: err.Error()})
		m.log.Warn("plugin instance not started", "section", kind.Section(), "key", inst.Key, "err", err)
		return Running{}, false
	}
	return Running{Key: inst.Key, Name: inst.Name, PluginID: inst.Type, Kind: kind, Settings: inst.Settings, Impl: impl}, true
}

func (m *Manager) construct(kind plugin.Kind, inst config.Instance) (any, error) {
	reg, ok := m.reg.Get(inst.Type)
	if !ok {
		return nil, errors.New("no plugin " + inst.Type)
	}
	impl, err := reg.New(plugin.Instance{Key: inst.Key, Name: inst.Name, PluginID: inst.Type, Settings: inst.Settings,
		Env: env{http: m.http.For(RefOf(kind, inst.Key))}})
	if err == nil && impl == nil {
		err = errors.New("plugin " + inst.Type + " built nothing")
	}
	return impl, err
}

func classOf(err error, def string) string {
	if c := plugin.ClassOf(err); c != "" {
		return string(c)
	}
	return def
}

// All returns the running instances of a kind, sorted by key.
func (m *Manager) All(kind plugin.Kind) []Running { return (*m.cur.Load())[kind] }

// Of returns the running instances of a kind that implement T.
func Of[T any](m *Manager, kind plugin.Kind) []Named[T] {
	var out []Named[T]
	for _, r := range m.All(kind) {
		if impl, ok := r.Impl.(T); ok {
			out = append(out, Named[T]{Running: r, Impl: impl})
		}
	}
	return out
}

// Get returns one instance of a kind implementing T.
func Get[T any](m *Manager, kind plugin.Kind, key domain.ObjectKey) (Named[T], bool) {
	for _, n := range Of[T](m, kind) {
		if n.Key == key {
			return n, true
		}
	}
	return Named[T]{}, false
}

// TestAll self-tests every instance except notifiers and reports health.
func (m *Manager) TestAll(ctx context.Context) error {
	for kind, list := range *m.cur.Load() {
		if kind == plugin.KindNotifier {
			continue
		}
		for _, r := range list {
			m.test(ctx, r)
		}
	}
	return nil
}

func (m *Manager) test(ctx context.Context, r Running) {
	t, ok := r.Impl.(plugin.Tester)
	if !ok {
		return
	}
	if err := t.Test(ctx); err != nil {
		m.rep.Report(r.Ref(), health.Observation{Class: classOf(err, health.ClassUnreachable), Message: err.Error()})
		return
	}
	m.rep.Report(r.Ref(), health.Observation{OK: true})
}
