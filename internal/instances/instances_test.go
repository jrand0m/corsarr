package instances

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/plugin"
)

type fakeIdx struct {
	err    error
	tested int
}

func (f *fakeIdx) Test(context.Context) error { f.tested++; return f.err }

type fakeNotifier struct{ fakeIdx }

type httpFor struct{ refs []domain.IntegrationRef }

func (h *httpFor) For(ref domain.IntegrationRef) plugin.HTTPDoer {
	h.refs = append(h.refs, ref)
	return http.DefaultClient
}

func registry(built map[string]any) *plugin.Registry {
	reg := plugin.NewRegistry()
	add := func(id string, k plugin.Kind) {
		reg.MustRegister(plugin.Registration{Manifest: plugin.Manifest{ID: id, Kind: k, APIVersion: plugin.APIVersion},
			New: func(inst plugin.Instance) (any, error) {
				if inst.Settings.String("fail") != "" {
					return nil, plugin.Errorf(plugin.ClassConfig, "bad settings")
				}
				if inst.Env.HTTP() == nil || inst.Env.Now().IsZero() {
					return nil, errors.New("no env")
				}
				return built[string(inst.Key)], nil
			}})
	}
	add("idx", plugin.KindIndexer)
	add("note", plugin.KindNotifier)
	return reg
}

func cfg(insts ...config.Instance) *config.Config {
	c := &config.Config{Instances: map[string]map[domain.ObjectKey]config.Instance{"bogus": {"x": {}}}}
	for _, in := range insts {
		if c.Instances[in.Section] == nil {
			c.Instances[in.Section] = map[domain.ObjectKey]config.Instance{}
		}
		c.Instances[in.Section][in.Key] = in
	}
	return c
}

func TestApplyBuildsTestsAndSwaps(t *testing.T) {
	good, bad, note := &fakeIdx{}, &fakeIdx{err: plugin.Errorf(plugin.ClassAuth, "nope")}, &fakeNotifier{}
	reg := registry(map[string]any{"b": good, "a": bad, "n": note})
	rep := health.New(nil, nil, nil)
	hf := &httpFor{}
	m := New(reg, hf, rep, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.Apply(cfg(
		config.Instance{Section: "indexers", Key: "b", Type: "idx", Enabled: true, Settings: plugin.Settings{}},
		config.Instance{Section: "indexers", Key: "a", Type: "idx", Enabled: true, Settings: plugin.Settings{}},
		config.Instance{Section: "indexers", Key: "off", Type: "idx", Enabled: false},
		config.Instance{Section: "indexers", Key: "broken", Type: "idx", Enabled: true, Settings: plugin.Settings{"fail": "y"}},
		config.Instance{Section: "indexers", Key: "ghost", Type: "missing", Enabled: true},
		config.Instance{Section: "indexers", Key: "nil", Type: "idx", Enabled: true, Settings: plugin.Settings{}},
		config.Instance{Section: "notifications", Key: "n", Type: "note", Enabled: true, Settings: plugin.Settings{"on": []any{"grab"}}},
	))
	idx := Of[plugin.Tester](m, plugin.KindIndexer)
	if len(idx) != 2 || idx[0].Key != "a" || idx[1].Key != "b" {
		t.Fatalf("sorted running indexers: %+v", idx)
	}
	if n, ok := Get[*fakeNotifier](m, plugin.KindNotifier, "n"); !ok || n.Settings.Strings("on")[0] != "grab" {
		t.Fatal("notifier with settings")
	}
	if _, ok := Get[*fakeNotifier](m, plugin.KindNotifier, "zz"); ok {
		t.Fatal("unknown key")
	}
	h, _ := rep.Get(RefOf(plugin.KindIndexer, "broken"))
	if h.Status != domain.HealthDegraded || h.LastErrorClass != "config" {
		t.Fatalf("broken instance health: %+v", h)
	}
	if h, _ := rep.Get(RefOf(plugin.KindIndexer, "off")); h.Status != domain.HealthDisabled {
		t.Fatalf("disabled: %+v", h)
	}
	_ = m.TestAll(context.Background())
	if good.tested != 1 || bad.tested != 1 || note.tested != 0 {
		t.Fatalf("tests: %d %d %d (notifiers never)", good.tested, bad.tested, note.tested)
	}
	if h, _ := rep.Get(RefOf(plugin.KindIndexer, "a")); h.Status != domain.HealthActionNeeded {
		t.Fatalf("auth failure: %+v", h)
	}
	if h, _ := rep.Get(RefOf(plugin.KindIndexer, "b")); h.Status != domain.HealthOK {
		t.Fatalf("ok: %+v", h)
	}
	m.Apply(cfg())
	if len(m.All(plugin.KindIndexer)) != 0 {
		t.Fatal("config change removes instances")
	}
}

func TestTestSkipsNonTesters(t *testing.T) {
	m := New(registry(map[string]any{"s": "not a tester"}), &httpFor{}, health.New(nil, nil, nil), slog.Default())
	m.Apply(cfg(config.Instance{Section: "indexers", Key: "s", Type: "idx", Enabled: true, Settings: plugin.Settings{}}))
	if err := m.TestAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if classOf(errors.New("x"), "d") != "d" {
		t.Fatal("default class")
	}
}

type closer struct {
	fakeIdx
	closed int
}

func (c *closer) Close() error { c.closed++; return errors.New("already gone") }

func TestApplyRebuildsOnlyChangedInstances(t *testing.T) {
	builds := map[domain.ObjectKey]int{}
	made := map[domain.ObjectKey][]*closer{} // by key: build order across kinds is not fixed
	reg := plugin.NewRegistry()
	for id, k := range map[string]plugin.Kind{"dl": plugin.KindDownloader, "note": plugin.KindNotifier} {
		reg.MustRegister(plugin.Registration{Manifest: plugin.Manifest{ID: id, Kind: k, APIVersion: plugin.APIVersion},
			New: func(inst plugin.Instance) (any, error) {
				builds[inst.Key]++
				c := &closer{}
				made[inst.Key] = append(made[inst.Key], c)
				return c, nil
			}})
	}
	m := New(reg, &httpFor{}, health.New(nil, nil, nil), slog.New(slog.NewTextHandler(io.Discard, nil)))
	qbit := config.Instance{Section: "downloadClients", Key: "qbit", Type: "dl", Enabled: true, Settings: plugin.Settings{"url": "http://q", "tags": []any{"a"}}}
	phone := config.Instance{Section: "notifications", Key: "phone", Type: "note", Enabled: true, Settings: plugin.Settings{"on": []any{"grab"}}}
	m.Apply(cfg(qbit, phone))
	before := m.All(plugin.KindDownloader)[0].Impl
	phone.Settings = plugin.Settings{"on": []any{"grab", "import"}} // a notification change
	m.Apply(cfg(qbit, phone))
	if m.All(plugin.KindDownloader)[0].Impl != before || builds["qbit"] != 1 || builds["phone"] != 2 {
		t.Fatalf("qbit rebuilt on a notification change: %v", builds)
	}
	if made["phone"][0].closed != 1 || made["qbit"][0].closed != 0 {
		t.Fatal("the replaced notifier is closed, the kept downloader not")
	}
	qbit.Name = "Renamed"
	m.Apply(cfg(qbit, phone))
	if builds["qbit"] != 2 {
		t.Fatal("a changed instance is rebuilt")
	}
	m.Apply(cfg(phone)) // qbit removed
	if len(m.All(plugin.KindDownloader)) != 0 || made["qbit"][1].closed != 1 {
		t.Fatal("a removed instance is closed")
	}
	phone.Enabled = false
	m.Apply(cfg(phone))
	if len(m.All(plugin.KindNotifier)) != 0 || made["phone"][1].closed != 1 {
		t.Fatal("a disabled instance is closed")
	}
	if !Same(Running{PluginID: "x", Settings: plugin.Settings{}}, config.Instance{Type: "x", Settings: plugin.Settings{}}) {
		t.Fatal("same config")
	}
}
