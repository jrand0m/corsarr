package main

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/wasm/wasmtest"
	"github.com/jrand0m/corsarr/plugins/example-downloader/fakeapi"
)

func writeModule(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPluginCommands(t *testing.T) {
	data := t.TempDir()
	dd := "--data.dir=" + data
	if code, _, errs := cliRun(t, nil, "plugin"); code != 2 || !strings.Contains(errs, "usage") {
		t.Fatalf("%d %s", code, errs)
	}
	if code, _, errs := cliRun(t, nil, "plugin", "load"); code != 2 || !strings.Contains(errs, "exactly one") {
		t.Fatalf("%d %s", code, errs)
	}
	if code, out, _ := cliRun(t, nil, "plugin", "list", dd); code != 0 || !strings.Contains(out, "qbittorrent") {
		t.Fatalf("%d %s", code, out)
	}
	mod := writeModule(t, "ex.wasm", wasmtest.Example(t))
	if code, _, errs := cliRun(t, nil, "plugin", "load", mod, dd, "--sha256=00"); code != 1 || !strings.Contains(errs, "checksum") {
		t.Fatalf("%d %s", code, errs)
	}
	code, out, errs := cliRun(t, nil, "plugin", "load", mod, dd)
	if code != 0 || !strings.Contains(out, "installed exampledl 1.0.0") || !strings.Contains(out, "may contact: $setting:url") {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if code, out, _ := cliRun(t, nil, "plugin", "list", dd); code != 0 || !strings.Contains(out, "exampledl") || !strings.Contains(out, "active") {
		t.Fatalf("%d %s", code, out)
	}
	if code, _, errs := cliRun(t, nil, "config", "validate", dd, "--downloadClients.ex.type=exampledl", "--downloadClients.ex.url=http://box:1"); code != 0 {
		t.Fatalf("config naming a module: %d %s", code, errs)
	}
	if code, out, _ := cliRun(t, nil, "plugin", "unload", "exampledl", dd); code != 0 || !strings.Contains(out, "unloaded") {
		t.Fatalf("%d %s", code, out)
	}
	if code, out, _ := cliRun(t, nil, "plugin", "list", dd); code != 0 || !strings.Contains(out, "unloaded") {
		t.Fatalf("%d %s", code, out)
	}
	if code, _, errs := cliRun(t, nil, "plugin", "rollback", "exampledl", dd); code != 1 || !strings.Contains(errs, "no previous") {
		t.Fatalf("%d %s", code, errs)
	}
	if code, _, _ := cliRun(t, nil, "plugin", "unload", "ghost", dd); code != 1 {
		t.Fatal("ghost unloaded")
	}
	if code, _, _ := cliRun(t, nil, "plugin", "load", "/no/such.wasm", dd); code != 1 {
		t.Fatal("missing file")
	}
	plugin.Default.Unregister("exampledl")
}

func TestPluginTest(t *testing.T) {
	api := httptest.NewServer(fakeapi.New("").Handler())
	defer api.Close()
	mod := writeModule(t, "ex.wasm", wasmtest.ExampleRaw(t))
	_ = os.WriteFile(strings.TrimSuffix(mod, ".wasm")+".json", wasmtest.ExampleManifest(t), 0o644)
	settings := writeModule(t, "s.json", []byte(`{"url":"`+api.URL+`"}`))
	code, out, errs := cliRun(t, nil, "plugin", "test", mod, "--settings="+settings)
	passed := code == 0 && strings.Contains(out, "all 3 checks passed")
	if !strings.Contains(out, "PASS  downloader conformance") || !(passed || raceEnabled) {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	if code, out, _ := cliRun(t, nil, "plugin", "test", mod); !(code == 0 || raceEnabled) || !strings.Contains(out, "live checks skipped") {
		t.Fatalf("%d %s", code, out)
	}
	down := writeModule(t, "s.json", []byte(`{"url":"http://127.0.0.1:1"}`))
	if code, out, _ := cliRun(t, nil, "plugin", "test", mod, "--settings="+down); code != 1 || !strings.Contains(out, "FAIL  downloader conformance") {
		t.Fatalf("%d %s", code, out)
	}
	badSettings := writeModule(t, "b.json", []byte(`nope`))
	if code, out, _ := cliRun(t, nil, "plugin", "test", mod, "--settings="+badSettings); code != 1 || !strings.Contains(out, "FAIL  settings") {
		t.Fatalf("%d %s", code, out)
	}
	bad := writeModule(t, "bad.wasm", []byte("\x00asm\x01\x00\x00\x00"))
	if code, out, _ := cliRun(t, nil, "plugin", "test", bad); code != 1 || !strings.Contains(out, "FAIL  load") {
		t.Fatalf("%d %s", code, out)
	}
	if code, _, _ := cliRun(t, nil, "plugin", "test", "/no/file.wasm"); code != 1 {
		t.Fatal("missing file")
	}
	if c := suite("indexer", struct{}{}); c.Passed {
		t.Error("kind without suite passed")
	}
}

func TestServerModules(t *testing.T) {
	data := t.TempDir()
	c := parse([]string{"--data.dir=" + data}, nil, os.Stdout, os.Stderr)
	c.prepareModules(slog.New(slog.DiscardHandler))
	if c.mods.mgr == nil {
		t.Fatal("manager not opened")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.runModules(ctx, nil)
	cancel()
	c.closeModules()
	if _, err := os.Stat(filepath.Join(data, "plugins")); err != nil {
		t.Error("plugins folder not created")
	}
	none := parse(nil, nil, os.Stdout, os.Stderr)
	none.runModules(context.Background(), nil)
	none.closeModules()
	bad := parse([]string{"--plugins.dir=relative"}, nil, os.Stdout, os.Stderr)
	bad.prepareModules(slog.New(slog.DiscardHandler))
	if limitsOf(config.Plugins{MaxModuleMiB: 2}).MaxModuleBytes != 2<<20 {
		t.Error("limits")
	}
}
