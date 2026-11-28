package modules

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/internal/wasm/wasmtest"
	"github.com/jrand0m/corsarr/plugins/example-downloader/fakeapi"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

func TestMain(m *testing.M) {
	code := m.Run()
	wasmtest.Cleanup()
	os.Exit(code)
}

type env struct{}

func (env) HTTP() plugin.HTTPDoer { return http.DefaultClient }
func (env) Now() time.Time        { return time.Now() }

func newManager(t *testing.T, dir string, reg *plugin.Registry, disabled ...string) *Manager {
	t.Helper()
	e, err := wasm.NewEngine(wasm.Options{CacheDir: wasmtest.CacheDir(), Compiler: true})
	if err != nil {
		t.Fatal(err)
	}
	m := New(Options{Dir: dir, Engine: e, Registry: reg, DisabledBuiltins: disabled})
	t.Cleanup(func() { m.Close(context.Background()); _ = e.Close(context.Background()) })
	return m
}

// exampleVersion is the example module with its manifest's version changed.
func exampleVersion(t *testing.T, version string) []byte {
	var man abi.Manifest
	if err := json.Unmarshal(wasmtest.ExampleManifest(t), &man); err != nil {
		t.Fatal(err)
	}
	man.Version = version
	b, _ := json.Marshal(man)
	return wasmtest.Embed(wasmtest.ExampleRaw(t), b)
}

func instance(t *testing.T, reg *plugin.Registry, url string) plugin.Downloader {
	t.Helper()
	r, ok := reg.Get("exampledl")
	if !ok {
		t.Fatal("not registered")
	}
	impl, err := r.New(plugin.Instance{Key: "dl", PluginID: "exampledl", Settings: plugin.Settings{"url": url}, Env: env{}})
	if err != nil {
		t.Fatal(err)
	}
	return impl.(plugin.Downloader)
}

func TestInstallRegisterAndReplace(t *testing.T) {
	dir, reg := t.TempDir(), plugin.NewRegistry()
	m := newManager(t, dir, reg)
	ctx := context.Background()
	v1 := wasmtest.Example(t)
	if _, err := m.Install(ctx, v1, nil, strings.Repeat("0", 64)); !errors.Is(err, ErrChecksum) {
		t.Fatalf("checksum: %v", err)
	}
	info, err := m.Install(ctx, v1, nil, wasm.SHA256(v1))
	if err != nil || info.Status != StatusActive || info.Version != "1.0.0" || len(info.Ops) == 0 {
		t.Fatalf("%+v %v", info, err)
	}
	r, _ := reg.Get("exampledl")
	if r.Manifest.Delivery != "module" || r.Manifest.Kind != plugin.KindDownloader || len(r.Manifest.Settings) != 4 {
		t.Errorf("%+v", r.Manifest)
	}
	srv := httptest.NewServer(fakeapi.New("").Handler())
	defer srv.Close()
	d := instance(t, reg, srv.URL)
	conformance.Downloader(t, d, plugin.AddRequest{Payload: plugin.Payload{Kind: "magnet", URL: "magnet:?xt=urn:btih:1"}, Category: "c"})
	if got, _ := m.Get("exampledl"); got.Instances != 1 {
		t.Errorf("instances %d", got.Instances)
	}

	if _, err := m.Install(ctx, exampleVersion(t, "1.1.0"), nil, ""); err != nil {
		t.Fatal(err)
	}
	h := d.(wasm.Handled).ModuleHandle()
	if v := h.Pool().Module().Manifest.Version; v != "1.1.0" {
		t.Errorf("running instance still on %s", v)
	}
	if err := d.Test(ctx); err != nil {
		t.Fatalf("after replace: %v", err)
	}
	info, _ = m.Get("exampledl")
	if info.Previous != "1.0.0" {
		t.Errorf("previous %q", info.Previous)
	}
	if info, err := m.Rollback("exampledl"); err != nil || info.Version != "1.0.0" || info.Previous != "1.1.0" {
		t.Fatalf("rollback %+v %v", info, err)
	}
	if v := h.Pool().Module().Manifest.Version; v != "1.0.0" {
		t.Errorf("rolled back instance on %s", v)
	}
	if err := d.(interface{ Close() error }).Close(); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Get("exampledl"); got.Instances != 0 {
		t.Errorf("closed instance still tracked: %d", got.Instances)
	}
}

func TestUnloadAndRemove(t *testing.T) {
	dir, reg := t.TempDir(), plugin.NewRegistry()
	m := newManager(t, dir, reg)
	if _, err := m.Install(context.Background(), wasmtest.Example(t), nil, ""); err != nil {
		t.Fatal(err)
	}
	d := instance(t, reg, "http://127.0.0.1:1")
	if err := m.Unload("exampledl"); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("exampledl"); ok {
		t.Error("still registered")
	}
	deadline := time.Now().Add(5 * time.Second)
	for d.Test(context.Background()) == nil || !strings.Contains(d.Test(context.Background()).Error(), "unloaded") {
		if time.Now().After(deadline) {
			t.Fatalf("instance still answers after unload: %v", d.Test(context.Background()))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if info, ok := m.Get("exampledl"); !ok || info.Status != StatusUnloaded || info.SHA256 == "" || info.Kind != "downloader" {
		t.Errorf("%+v", info)
	}
	if err := m.Unload("ghost"); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("ghost: %v", err)
	}
	if err := m.Remove("exampledl"); err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 0 {
		t.Errorf("%+v", m.List())
	}
	if _, err := m.Rollback("exampledl"); err == nil {
		t.Error("rollback of a removed module")
	}
	if _, err := m.Module("exampledl"); !errors.Is(err, ErrNotInstalled) {
		t.Error(err)
	}
}

func TestBuiltinConflict(t *testing.T) {
	builtin := plugin.Registration{Manifest: plugin.Manifest{ID: "exampledl", Kind: plugin.KindDownloader, APIVersion: plugin.APIVersion},
		New: func(plugin.Instance) (any, error) { return nil, nil }}
	reg := plugin.NewRegistry()
	reg.MustRegister(builtin)
	m := newManager(t, t.TempDir(), reg)
	if _, err := m.Install(context.Background(), wasmtest.Example(t), nil, ""); !errors.Is(err, ErrBuiltin) {
		t.Fatalf("built-in id taken: %v", err)
	}
	reg2 := plugin.NewRegistry()
	reg2.MustRegister(builtin)
	m2 := newManager(t, t.TempDir(), reg2, "exampledl")
	if _, ok := reg2.Get("exampledl"); ok {
		t.Fatal("disabled built-in still registered")
	}
	if _, err := m2.Install(context.Background(), wasmtest.Example(t), nil, ""); err != nil {
		t.Fatal(err)
	}
	if r, _ := reg2.Get("exampledl"); r.Manifest.Delivery != "module" {
		t.Error("module not registered")
	}
	_ = m2.Unload("exampledl")
	if r, ok := reg2.Get("exampledl"); !ok || r.Manifest.Delivery == "module" {
		t.Error("built-in not restored after unload")
	}
}

func TestRestartRegistersAndChecksHash(t *testing.T) {
	dir := t.TempDir()
	m := newManager(t, dir, plugin.NewRegistry())
	if _, err := m.Install(context.Background(), wasmtest.Example(t), nil, ""); err != nil {
		t.Fatal(err)
	}
	reg := plugin.NewRegistry()
	again := newManager(t, dir, reg)
	again.Register()
	if _, ok := reg.Get("exampledl"); !ok {
		t.Fatal("not registered after restart")
	}
	again.Warm()
	if info, _ := again.Get("exampledl"); info.Status != StatusActive || info.StartTime == 0 {
		t.Errorf("%+v", info)
	}
	path := filepath.Join(dir, "exampledl", "1.0.0", fileModule)
	b, _ := os.ReadFile(path)
	b[len(b)-1] ^= 0xff
	_ = os.WriteFile(path, b, 0o644)
	reg3 := plugin.NewRegistry()
	tampered := newManager(t, dir, reg3)
	tampered.Register()
	info, _ := tampered.Get("exampledl")
	if _, ok := reg3.Get("exampledl"); ok || info.Status != StatusFailed || !strings.Contains(info.Error, "checksum mismatch") {
		t.Errorf("tampered module: %+v", info)
	}
}

func TestDropInsAndStoreFollow(t *testing.T) {
	dir, reg := t.TempDir(), plugin.NewRegistry()
	m := newManager(t, dir, reg)
	_ = os.WriteFile(filepath.Join(dir, "good.wasm"), wasmtest.ExampleRaw(t), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "good.json"), wasmtest.ExampleManifest(t), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "bad.wasm"), []byte("not wasm"), 0o644)
	m.Sync(context.Background())
	if _, ok := reg.Get("exampledl"); !ok {
		t.Fatal("drop-in not installed")
	}
	if _, err := os.Stat(filepath.Join(dir, "good.wasm")); !os.IsNotExist(err) {
		t.Error("drop-in left behind")
	}
	reason, _ := os.ReadFile(filepath.Join(dir, "bad.wasm.rejected.txt"))
	if !strings.Contains(string(reason), "manifest") {
		t.Errorf("rejection reason %q", reason)
	}
	// another process (the CLI) deactivates; the server follows on sync
	other := newManager(t, dir, plugin.NewRegistry())
	if err := other.Store().Deactivate("exampledl"); err != nil {
		t.Fatal(err)
	}
	m.Sync(context.Background())
	if _, ok := reg.Get("exampledl"); ok {
		t.Error("not unloaded after the store changed")
	}
	if err := other.Store().Activate("exampledl", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	m.Sync(context.Background())
	if _, ok := reg.Get("exampledl"); !ok {
		t.Error("not loaded after the store changed")
	}
	_ = os.RemoveAll(filepath.Join(dir, "exampledl"))
	m.Sync(context.Background())
	if _, ok := reg.Get("exampledl"); ok {
		t.Error("removed module still registered")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m.Run(ctx)
}

func TestManagerAction(t *testing.T) {
	m := newManager(t, t.TempDir(), plugin.NewRegistry())
	if _, err := m.Install(context.Background(), wasmtest.Example(t), nil, ""); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(fakeapi.New("").Handler())
	defer srv.Close()
	res, err := m.Action(context.Background(), "exampledl", "test", map[string]any{"url": srv.URL}, http.DefaultClient)
	if err != nil || res.Outcome != abi.OutcomeOK {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := m.Action(context.Background(), "ghost", "test", nil, nil); err == nil {
		t.Error("ghost action")
	}
}

func TestStoreEdges(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	v := Version{ID: "a", Version: "1.0.0", SHA256: "x", Module: []byte("m")}
	if err := s.Write(v); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(v); err != nil {
		t.Error("identical rewrite refused")
	}
	v.SHA256 = "y"
	if err := s.Write(v); err == nil {
		t.Error("same version, different content accepted")
	}
	if _, err := s.Swap("a"); err == nil {
		t.Error("swap without previous")
	}
	if _, err := s.Read("a", "1.0.0", func([]byte) string { return "z" }); !errors.Is(err, ErrChecksum) {
		t.Error(err)
	}
	if _, err := s.Read("a", "9", wasm.SHA256); err == nil {
		t.Error("missing version")
	}
	if err := s.Deactivate("a"); err != nil {
		t.Error(err)
	}
	_ = s.Activate("a", "1.0.0")
	_ = os.MkdirAll(s.verDir("a", "2.0.0"), 0o755)
	_ = s.Activate("a", "2.0.0")
	if got, err := s.Swap("a"); err != nil || got != "1.0.0" {
		t.Errorf("%s %v", got, err)
	}
	if short("abc") != "abc" || s.CacheDir() == "" {
		t.Error("helpers")
	}
	_ = s.Deactivate("a")
	if got, err := s.Swap("a"); err != nil || got != "2.0.0" {
		t.Errorf("swap with no active: %s %v", got, err)
	}
	cached := filepath.Join(s.CacheDir(), "engine", "abc")
	_ = os.MkdirAll(filepath.Dir(cached), 0o755)
	_ = os.WriteFile(cached, []byte("code"), 0o644)
	_ = os.MkdirAll(s.verDir("a", "3.0.0"), 0o755)
	_ = s.Activate("a", "3.0.0")
	if _, err := os.Stat(cached); !os.IsNotExist(err) {
		t.Error("compiled code of a pruned version kept")
	}
	if _, err := os.Stat(filepath.Dir(cached)); err != nil {
		t.Error("cache directory removed")
	}
	if vs := s.versions("a"); len(vs) != 2 {
		t.Errorf("versions after prune: %v", vs)
	}
}
