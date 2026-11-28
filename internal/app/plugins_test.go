package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/catalogue"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/modules"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/internal/wasm/wasmtest"
	"github.com/jrand0m/corsarr/plugins/example-downloader/fakeapi"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// muxMounter mounts module routes on a plain mux (no auth) for tests.
type muxMounter struct{ *http.ServeMux }

func (m muxMounter) Handle(pattern string, h http.HandlerFunc) { m.HandleFunc(pattern, h) }

type eventLog struct {
	mu  sync.Mutex
	evs []domain.Event
}

func (l *eventLog) Publish(ev domain.Event) (domain.Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.evs = append(l.evs, ev)
	return ev, nil
}

func (l *eventLog) changes() []PluginChange {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []PluginChange
	for _, ev := range l.evs {
		if ev.Type == domain.EvPluginChanged {
			out = append(out, ev.Data.(PluginChange))
		}
	}
	return out
}

// testCatalogue serves a signed index for the modules it is given.
type testCatalogue struct {
	*httptest.Server
	mu      sync.Mutex
	files   map[string][]byte
	priv    ed25519.PrivateKey
	tamper  bool
	release string
}

func newTestCatalogue(t *testing.T) *testCatalogue {
	c := &testCatalogue{files: map[string][]byte{}, priv: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, 32)), release: "v1.4.0"}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		b, ok := c.files[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if c.tamper && strings.HasSuffix(r.URL.Path, ".wasm") {
			b = append([]byte{}, b...)
			b[len(b)/2] ^= 0xff
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *testCatalogue) key() catalogue.Key {
	pub := c.priv.Public().(ed25519.PublicKey)
	return catalogue.Key{ID: catalogue.KeyID(pub), Public: pub}
}

// offer publishes modules (each with its manifest embedded) as the index.
func (c *testCatalogue) offer(t *testing.T, mods ...[]byte) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	ix := catalogue.Index{Schema: 1, Release: c.release, Generated: time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)}
	for _, mod := range mods {
		raw, _ := wasm.CustomSection(mod, abi.ManifestSection)
		var m abi.Manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		name := m.ID + "-" + m.Version + ".wasm"
		c.files[name] = mod
		ix.Plugins = append(ix.Plugins, catalogue.Entry{ID: m.ID, Name: m.Name, Version: m.Version, Kind: m.Kind, ABIVersion: m.ABIVersion,
			Description: m.Description, Author: m.Author, License: m.License, Homepage: m.Homepage, Permissions: m.Permissions,
			MinCorsarr: "1.0.0", Size: int64(len(mod)), SHA256: wasm.SHA256(mod), URL: name})
	}
	b, _ := json.Marshal(ix)
	c.files["plugins-index.json"] = b
	c.files["plugins-index.json.sig"] = catalogue.Sign(b, c.priv)
}

func exampleAt(t *testing.T, version string, mutate func(*abi.Manifest)) []byte {
	t.Helper()
	var m abi.Manifest
	if err := json.Unmarshal(wasmtest.ExampleManifest(t), &m); err != nil {
		t.Fatal(err)
	}
	m.Version = version
	if mutate != nil {
		mutate(&m)
	}
	b, _ := json.Marshal(m)
	return wasmtest.Embed(wasmtest.ExampleRaw(t), b)
}

type adminFixture struct {
	admin   *PluginAdmin
	mux     *http.ServeMux
	cat     *testCatalogue
	cfg     *config.Config
	reg     *plugin.Registry
	events  *eventLog
	reloads []string
}

func newAdminFixture(t *testing.T) *adminFixture {
	t.Helper()
	dir := t.TempDir()
	eng, err := wasm.NewEngine(wasm.Options{CacheDir: wasmtest.CacheDir(), Compiler: true})
	if err != nil {
		t.Fatal(err)
	}
	reg := plugin.NewRegistry()
	mgr := modules.New(modules.Options{Dir: dir, Engine: eng, Registry: reg})
	t.Cleanup(func() { mgr.Close(context.Background()); _ = eng.Close(context.Background()) })
	cat := newTestCatalogue(t)
	f := &adminFixture{cat: cat, reg: reg, events: &eventLog{}, mux: http.NewServeMux()}
	f.cfg = &config.Config{Plugins: config.Plugins{Dir: dir, UIInstall: true, MaxModuleMiB: 32, AutoUpdate: "off",
		CatalogueURL: cat.URL + "/plugins-index.json"},
		Instances: map[string]map[domain.ObjectKey]config.Instance{"downloadClients": {
			"box": {Section: "downloadClients", Key: "box", Type: "exampledl", Name: "Box", Enabled: true, Settings: plugin.Settings{"url": "http://127.0.0.1:1"}},
		}}}
	f.admin = NewPluginAdmin(PluginAdminDeps{Modules: mgr, Config: func() *config.Config { return f.cfg }, HTTP: http.DefaultClient,
		Actions: func(string) plugin.HTTPDoer { return http.DefaultClient }, Bus: f.events,
		Reload: func(id string) { f.reloads = append(f.reloads, id) }, Keys: []catalogue.Key{cat.key()}, Version: "v1.4.0",
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	f.admin.Mount(muxMounter{f.mux})
	return f
}

func (f *adminFixture) do(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, httptest.NewRequest(method, path, rd))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func errCode(out map[string]any) string {
	e, _ := out["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func offerState(t *testing.T, f *adminFixture, refresh bool) string {
	t.Helper()
	path := "/api/v1/plugins/catalogue"
	if refresh {
		path += "?refresh=1"
	}
	code, out := f.do(t, "GET", path, nil)
	offers, _ := out["offers"].([]any)
	if code != 200 || len(offers) != 1 {
		t.Fatalf("catalogue %d %v", code, out)
	}
	return offers[0].(map[string]any)["state"].(string)
}

func TestPluginLifecycleWithoutRestart(t *testing.T) {
	f := newAdminFixture(t)
	v1, v2 := exampleAt(t, "1.0.0", nil), exampleAt(t, "1.1.0", nil)
	f.cat.offer(t, v1)

	if s := offerState(t, f, false); s != string(catalogue.StateAvailable) {
		t.Fatalf("state %s", s)
	}
	if code, out := f.do(t, "POST", "/api/v1/plugins/modules", installBody{ID: "exampledl", Version: "0.9.0"}); code != 409 || errCode(out) != "catalogue_changed" {
		t.Fatalf("other version: %d %v", code, out)
	}
	code, out := f.do(t, "POST", "/api/v1/plugins/modules", installBody{ID: "exampledl", Version: "1.0.0"})
	if code != 201 || out["status"] != "active" || out["source"] != "catalogue" || out["configSection"] != "downloadClients" {
		t.Fatalf("install: %d %v", code, out)
	}
	if used, _ := out["usedBy"].([]any); len(used) != 1 {
		t.Fatalf("usedBy %v", out["usedBy"])
	}
	if _, ok := f.reg.Get("exampledl"); !ok {
		t.Fatal("not registered after install")
	}

	f.cat.offer(t, v2)
	if s := offerState(t, f, true); s != string(catalogue.StateUpdate) {
		t.Fatalf("state after a new release %s", s)
	}
	if code, out := f.do(t, "POST", "/api/v1/plugins/modules/exampledl/update", map[string]string{"version": "1.1.0"}); code != 200 || out["version"] != "1.1.0" {
		t.Fatalf("update: %d %v", code, out)
	}
	if code, out := f.do(t, "POST", "/api/v1/plugins/modules/exampledl/rollback", nil); code != 200 || out["version"] != "1.0.0" || out["source"] != "catalogue" {
		t.Fatalf("rollback: %d %v", code, out)
	}
	code, out = f.do(t, "DELETE", "/api/v1/plugins/modules/exampledl", nil)
	if used, _ := out["usedBy"].([]any); code != 200 || len(used) != 1 {
		t.Fatalf("uninstall: %d %v", code, out)
	}
	if code, out := f.do(t, "GET", "/api/v1/plugins/modules/exampledl", nil); code != 200 || out["status"] != "uninstalled" {
		t.Fatalf("after uninstall: %d %v", code, out)
	}
	if s := offerState(t, f, false); s != string(catalogue.StateAvailable) {
		t.Fatalf("state after uninstall %s", s)
	}
	if code, out := f.do(t, "POST", "/api/v1/plugins/modules", installBody{ID: "exampledl", Version: "1.1.0"}); code != 201 || out["status"] != "active" {
		t.Fatalf("reinstall: %d %v", code, out)
	}

	var actions []string
	for _, c := range f.events.changes() {
		if c.ID != "exampledl" || c.Version == "" || c.SHA256 == "" {
			t.Fatalf("change without version or hash: %+v", c)
		}
		actions = append(actions, c.Action)
	}
	want := []string{ChangeInstalled, ChangeUpdated, ChangeRolledBack, ChangeUninstalled, ChangeInstalled}
	if strings.Join(actions, ",") != strings.Join(want, ",") || len(f.reloads) != len(want) {
		t.Fatalf("audit %v reloads %v", actions, f.reloads)
	}
}

func TestPluginInstallRefusals(t *testing.T) {
	f := newAdminFixture(t)
	f.cat.offer(t, exampleAt(t, "1.0.0", nil))
	f.cat.tamper = true
	if code, out := f.do(t, "POST", "/api/v1/plugins/modules", installBody{ID: "exampledl", Version: "1.0.0"}); code != 422 || errCode(out) != "verify_failed" {
		t.Fatalf("tampered: %d %v", code, out)
	}
	if _, out := f.do(t, "GET", "/api/v1/plugins/modules", nil); len(out["items"].([]any)) != 0 {
		t.Fatalf("something was installed: %v", out)
	}
	f.cat.tamper = false

	f.cat.release = "v9.0.0"
	newer := exampleAt(t, "2.0.0", nil)
	f.cat.offer(t, newer)
	f.cat.mu.Lock()
	var ix catalogue.Index
	_ = json.Unmarshal(f.cat.files["plugins-index.json"], &ix)
	ix.Plugins[0].MinCorsarr = "9.0.0"
	b, _ := json.Marshal(ix)
	f.cat.files["plugins-index.json"], f.cat.files["plugins-index.json.sig"] = b, catalogue.Sign(b, f.cat.priv)
	f.cat.mu.Unlock()
	if s := offerState(t, f, true); s != string(catalogue.StateIncompatible) {
		t.Fatalf("state %s", s)
	}
	if code, out := f.do(t, "POST", "/api/v1/plugins/modules", installBody{ID: "exampledl", Version: "2.0.0"}); code != 422 || errCode(out) != "incompatible" {
		t.Fatalf("incompatible: %d %v", code, out)
	}
	if code, _ := f.do(t, "POST", "/api/v1/plugins/modules", installBody{ID: "ghost", Version: "1.0.0"}); code != 404 {
		t.Fatalf("ghost: %d", code)
	}
}

func TestPluginLockRefusesEveryChange(t *testing.T) {
	f := newAdminFixture(t)
	f.cat.offer(t, exampleAt(t, "1.0.0", nil))
	if code, _ := f.do(t, "POST", "/api/v1/plugins/modules", installBody{ID: "exampledl", Version: "1.0.0"}); code != 201 {
		t.Fatal("install")
	}
	f.cfg.Plugins.UIInstall = false
	for _, r := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/v1/plugins/modules", installBody{ID: "exampledl", Version: "1.0.0"}},
		{"POST", "/api/v1/plugins/modules/exampledl/update", map[string]string{"version": "1.0.0"}},
		{"POST", "/api/v1/plugins/modules/exampledl/rollback", nil},
		{"DELETE", "/api/v1/plugins/modules/exampledl", nil},
	} {
		code, out := f.do(t, r.method, r.path, r.body)
		if code != 403 || errCode(out) != "plugins_locked" || !strings.Contains(out["error"].(map[string]any)["message"].(string), "plugins.uiInstall") {
			t.Fatalf("%s %s: %d %v", r.method, r.path, code, out)
		}
	}
	code, out := f.do(t, "GET", "/api/v1/plugins/catalogue", nil)
	if code != 200 || out["policy"].(map[string]any)["uiInstall"] != false {
		t.Fatalf("catalogue stays readable: %d %v", code, out)
	}
	if info, _ := f.admin.Get("exampledl"); info.Status != modules.StatusActive {
		t.Fatal("the module changed while locked")
	}
}

func TestPluginActionsAndValidation(t *testing.T) {
	f := newAdminFixture(t)
	f.cat.offer(t, exampleAt(t, "1.0.0", nil))
	f.cfg.Plugins.UIInstall = false // actions and validation change nothing, so the lock allows them
	if _, err := f.admin.installFromCatalogue(context.Background(), "exampledl", "1.0.0", "system"); err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(fakeapi.New("").Handler())
	defer api.Close()
	code, out := f.do(t, "POST", "/api/v1/plugins/modules/exampledl/actions/test", valuesBody{Values: map[string]any{"url": api.URL}})
	if code != 200 || out["outcome"] != abi.OutcomeOK {
		t.Fatalf("draft action: %d %v", code, out)
	}
	f.cfg.Instances["downloadClients"]["box"] = config.Instance{Section: "downloadClients", Key: "box", Type: "exampledl", Enabled: true, Settings: plugin.Settings{"url": api.URL}}
	if code, out := f.do(t, "POST", "/api/v1/plugins/modules/exampledl/actions/test", valuesBody{Instance: "downloadClients.box", Values: map[string]any{"url": ""}}); code != 200 || out["outcome"] != abi.OutcomeOK {
		t.Fatalf("instance action: %d %v", code, out)
	}
	if code, _ := f.do(t, "POST", "/api/v1/plugins/modules/exampledl/actions/test", valuesBody{Instance: "downloadClients.ghost"}); code != 400 {
		t.Fatalf("unknown instance: %d", code)
	}
	code, out = f.do(t, "POST", "/api/v1/plugins/modules/exampledl/actions/test", valuesBody{Values: map[string]any{"url": "not a url"}})
	if probs, _ := out["problems"].([]any); code != 422 || len(probs) == 0 || errCode(out) != "invalid_values" {
		t.Fatalf("invalid values: %d %v", code, out)
	}
	if code, _ := f.do(t, "POST", "/api/v1/plugins/modules/exampledl/actions/nope", valuesBody{Values: map[string]any{"url": api.URL}}); code != 404 {
		t.Fatalf("unknown action: %d", code)
	}
	code, out = f.do(t, "POST", "/api/v1/plugins/modules/exampledl/validate", valuesBody{Values: map[string]any{"timeout": "2h"}})
	if probs, _ := out["problems"].([]any); code != 200 || len(probs) < 2 { // url required, timeout too long
		t.Fatalf("validate: %d %v", code, out)
	}
	if code, out := f.do(t, "POST", "/api/v1/plugins/modules/exampledl/validate", valuesBody{Values: map[string]any{"url": api.URL}}); code != 200 || len(out["problems"].([]any)) != 0 {
		t.Fatalf("valid draft: %d %v", code, out)
	}
}

func TestPluginAutoUpdate(t *testing.T) {
	f := newAdminFixture(t)
	f.cat.offer(t, exampleAt(t, "1.0.0", nil))
	ctx := context.Background()
	if _, err := f.admin.installFromCatalogue(ctx, "exampledl", "1.0.0", "user"); err != nil {
		t.Fatal(err)
	}
	grown := exampleAt(t, "1.0.1", func(m *abi.Manifest) { m.Permissions.HTTP.Hosts = append(m.Permissions.HTTP.Hosts, "example.org") })
	f.cat.offer(t, grown)
	f.cfg.Plugins.AutoUpdate = catalogue.AutoPatch
	if err := f.admin.AutoUpdate(ctx); err != nil {
		t.Fatal(err)
	}
	if info, _ := f.admin.Get("exampledl"); info.Version != "1.0.0" {
		t.Fatal("a version asking for new permissions was installed on its own")
	}
	f.cat.offer(t, exampleAt(t, "1.0.2", nil))
	if err := f.admin.AutoUpdate(ctx); err != nil {
		t.Fatal(err)
	}
	if info, _ := f.admin.Get("exampledl"); info.Version != "1.0.2" {
		t.Fatalf("patch update not applied: %s", info.Version)
	}
	f.cat.offer(t, exampleAt(t, "1.1.0", nil))
	if err := f.admin.AutoUpdate(ctx); err != nil {
		t.Fatal(err)
	}
	if info, _ := f.admin.Get("exampledl"); info.Version != "1.0.2" {
		t.Fatal("a minor update was applied under the patch policy")
	}
	changes := f.events.changes()
	if last := changes[len(changes)-1]; last.Action != ChangeUpdated || last.Previous != "1.0.0" {
		t.Fatalf("audit %+v", last)
	}
	if f.events.evs[len(f.events.evs)-1].Actor != "system" {
		t.Fatal("auto-update actor")
	}
}

func TestPluginAdminWithoutModules(t *testing.T) {
	cfg := &config.Config{Plugins: config.Plugins{Dir: t.TempDir(), UIInstall: true}}
	a := NewPluginAdmin(PluginAdminDeps{Config: func() *config.Config { return cfg }, HTTP: http.DefaultClient, Version: "dev"})
	mux := http.NewServeMux()
	a.Mount(muxMounter{mux})
	for _, path := range []string{"/api/v1/plugins/modules", "/api/v1/plugins/modules/x"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 503 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	if a.Policy().CatalogueURL != catalogue.DefaultURL("dev") {
		t.Fatal("default catalogue")
	}
	cfg.Plugins.CatalogueURL = "http://127.0.0.1:1/plugins-index.json" // nothing listens: no test reaches the internet
	if err := a.AutoUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := a.Catalogue(context.Background(), false); v.Source.Error == "" {
		t.Fatal("no keys: the catalogue must say why it cannot be read")
	}
}

func TestMCPPluginList(t *testing.T) {
	f := newAdminFixture(t)
	f.cat.offer(t, exampleAt(t, "1.0.0", nil))
	if _, err := f.admin.installFromCatalogue(context.Background(), "exampledl", "1.0.0", "user"); err != nil {
		t.Fatal(err)
	}
	f.cat.offer(t, exampleAt(t, "1.0.1", nil))
	_ = f.admin.Catalogue(context.Background(), true)
	a := &App{d: Deps{Config: func() *config.Config { return f.cfg }, Plugins: f.reg}, Plugins: f.admin}
	list := mcpPlugins{a}.PluginList()
	if len(list) != 1 || list[0].Delivery != "module" || list[0].Status != "active" || list[0].Update != "1.0.1" ||
		len(list[0].Instances) != 1 || list[0].Hosts[0] != "$setting:url" {
		t.Fatalf("%+v", list)
	}
}
