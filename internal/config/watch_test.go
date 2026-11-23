package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
)

func testRegistryEmpty() *plugin.Registry { return plugin.NewRegistry() }

func TestPluginFieldTypes(t *testing.T) {
	reg := testRegistry(t)
	reg.MustRegister(plugin.Registration{New: func(plugin.Instance) (any, error) { return nil, nil }, Manifest: plugin.Manifest{
		ID: "plex", Kind: plugin.KindMediaServer, APIVersion: plugin.APIVersion,
		Settings: []plugin.Field{
			{Key: "libs", Type: plugin.FieldMultiselect, Options: []string{"movies", "tv"}},
			{Key: "headers", Type: plugin.FieldMap},
			{Key: "ssl", Type: plugin.FieldBool, Default: false},
			{Key: "mode", Type: plugin.FieldEnum, Options: []string{"a", "b"}, Default: "a"},
			{Key: "max", Type: plugin.FieldNumber, Max: f64(5)},
		},
	}})
	ok := []string{"--mediaServers.plex.type=plex", "--mediaServers.plex.libs.0=movies", "--mediaServers.plex.headers.xtoken=v", "--mediaServers.plex.ssl=true", "--mediaServers.plex.mode=b"}
	c, err := Load(inputs(ok, nil, nil), reg)
	if err != nil {
		t.Fatal(err)
	}
	s := c.Instances["mediaServers"]["plex"].Settings
	if s["ssl"] != true || s["mode"] != "b" || s["headers"].(map[string]any)["xtoken"] != "v" || s["libs"].([]any)[0] != "movies" {
		t.Fatalf("settings: %v", s)
	}
	bad := []string{"--mediaServers.plex.type=plex", "--mediaServers.plex.libs.0=music", "--mediaServers.plex.ssl=maybe", "--mediaServers.plex.max=x"}
	_, err = Load(inputs(bad, nil, nil), reg)
	for _, frag := range []string{"must be one of movies, tv", "must be true or false", "must be a number"} {
		if err == nil || !strings.Contains(err.Error(), frag) {
			t.Errorf("missing %q: %v", frag, err)
		}
	}
	if _, err := Load(inputs([]string{"--mediaServers.plex.type=plex", "--mediaServers.plex.max=9"}, nil, nil), reg); err == nil || !strings.Contains(err.Error(), "≤ 5") {
		t.Errorf("max: %v", err)
	}
	if _, err := Load(inputs([]string{"--version=x"}, nil, nil), reg); err == nil || !strings.Contains(err.Error(), "integer") {
		t.Errorf("int: %v", err)
	}
	if _, err := Load(inputs([]string{"--indexers.k.type=torznab", "--indexers.k.url=u", "--indexers.k.timeout=soon"}, nil, nil), reg); err == nil || !strings.Contains(err.Error(), "duration") {
		t.Errorf("duration: %v", err)
	}
	if _, err := Load(inputs([]string{"--server.basePath=x/"}, nil, nil), reg); err == nil {
		t.Error("basePath")
	}
}

func TestDebounceFiresOncePerSettledChange(t *testing.T) {
	a := fileSig{target: "/a", size: 1, exists: true}
	b := fileSig{target: "/a", size: 2, exists: true}
	c := fileSig{target: "/b", size: 2, exists: true}
	d := &debounce{last: a}
	steps := []struct {
		sig  fileSig
		fire bool
	}{
		{a, false}, {b, false}, {c, false}, {c, true}, {c, false}, {a, false}, {c, false}, {fileSig{}, false}, {fileSig{}, true},
	}
	for i, s := range steps {
		if got := d.step(s.sig); got != s.fire {
			t.Fatalf("step %d: fire=%v want %v", i, got, s.fire)
		}
	}
}

func TestStatSigFollowsSymlinkSwap(t *testing.T) {
	dir := t.TempDir()
	a, b, link := filepath.Join(dir, "a.yml"), filepath.Join(dir, "b.yml"), filepath.Join(dir, "corsarr.yml")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(a, link); err != nil {
		t.Fatal(err)
	}
	s1 := statSig(link)
	_ = os.Remove(link)
	_ = os.Symlink(b, link)
	if s2 := statSig(link); s1 == s2 || !s2.exists {
		t.Fatalf("swap not detected: %+v %+v", s1, s2)
	}
	if statSig(filepath.Join(dir, "missing")).exists {
		t.Fatal("missing file exists")
	}
}

func TestWatchFileAppliesLiveChangesAndFlagsBootstrap(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "corsarr.yml")
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		future := time.Now().Add(time.Duration(len(s)) * time.Second)
		_ = os.Chtimes(p, future, future)
	}
	write("log:\n  level: info\n")
	in := OSInputs([]string{"--configFile=" + p})
	cur, err := Load(in, testRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan Change, 4)
	go WatchFile(ctx, cur, in, testRegistry(t), 10*time.Millisecond, func(c Change) { changes <- c })
	time.Sleep(50 * time.Millisecond) // let the watcher take its first signature

	write("log:\n  level: debug\nmediaKinds:\n  book:\n    mode: active\n")
	ch := next(t, changes)
	if ch.Err != nil || strings.Join(ch.Bootstrap, ",") != "log" || strings.Join(ch.Live, ",") != "mediaKinds" {
		t.Fatalf("change: %+v", ch)
	}
	write("log:\n  level: nope\n")
	if ch := next(t, changes); ch.Err == nil || ch.Config != nil {
		t.Fatalf("invalid file should report errors: %+v", ch)
	}
	_ = os.Remove(p)
	if ch := next(t, changes); !ch.Missing {
		t.Fatalf("missing: %+v", ch)
	}
}

func next(t *testing.T, ch chan Change) Change {
	t.Helper()
	select {
	case c := <-ch:
		return c
	case <-time.After(3 * time.Second):
		t.Fatal("no change observed")
	}
	return Change{}
}

func TestWatchFileIgnoresNonFileSources(t *testing.T) {
	c := mustLoad(t, inputs(nil, nil, nil))
	done := make(chan struct{})
	go func() {
		WatchFile(context.Background(), c, Inputs{}, testRegistry(t), time.Millisecond, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WatchFile should return at once for non-file sources")
	}
}

func TestDiffDetectsSecretValueChanges(t *testing.T) {
	a := mustLoad(t, inputs(nil, sameEnv, nil))
	env := append([]string{}, sameEnv...)
	for i, kv := range env {
		if strings.HasPrefix(kv, "CORSARR_INDEXERS_KNABEN_APIKEY=") {
			env[i] = "CORSARR_INDEXERS_KNABEN_APIKEY=rotated"
		}
	}
	b := mustLoad(t, inputs(nil, env, nil))
	boot, live := Diff(a, b)
	if len(boot) != 0 || strings.Join(live, ",") != "indexers" {
		t.Fatalf("diff: %v %v", boot, live)
	}
	if boot, live := Diff(a, a); len(boot)+len(live) != 0 {
		t.Fatal("self diff")
	}
}

func TestJSONSchemaDescribesTheTree(t *testing.T) {
	s := NewSchema(testRegistry(t))
	b := s.JSONSchema()
	if string(b) != string(s.JSONSchema()) {
		t.Fatal("not deterministic")
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	props := doc["properties"].(map[string]any)
	idx := props["indexers"].(map[string]any)
	variants := idx["additionalProperties"].(map[string]any)["oneOf"].([]any)
	tz := variants[0].(map[string]any)
	if tz["properties"].(map[string]any)["type"].(map[string]any)["const"] != "torznab" {
		t.Fatalf("variant: %v", tz)
	}
	for _, want := range []string{`"writeOnly": true`, `"default": ":8090"`, `"pattern": "^[a-z][a-z0-9]*$"`, `"default": "30s"`, `"default": 1`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("schema misses %s", want)
		}
	}
	empty := NewSchema(testRegistryEmpty()).JSONSchema()
	if !strings.Contains(string(empty), `"required": [`) {
		t.Fatal("empty registry instance schema")
	}
}
