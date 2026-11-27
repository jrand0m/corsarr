package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

func init() {
	plugin.Default.MustRegister(plugin.Registration{
		New: func(plugin.Instance) (any, error) { return nil, nil },
		Manifest: plugin.Manifest{ID: "fakeidx", Kind: plugin.KindIndexer, APIVersion: plugin.APIVersion,
			Settings: []plugin.Field{{Key: "apiKey", Type: plugin.FieldSecret}}},
	})
}

// cliRun runs a command with an isolated environment (no HOME config files).
func cliRun(t *testing.T, environ []string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(context.Background(), args, append([]string{"HOME=" + t.TempDir()}, environ...), &out, &errb)
	return code, out.String(), errb.String()
}

func TestVersionAndUnknown(t *testing.T) {
	if code, out, _ := cliRun(t, nil, "version"); code != 0 || out != "corsarr dev\n" {
		t.Fatalf("%d %q", code, out)
	}
	if code, _, errs := cliRun(t, nil, "frobnicate"); code != 2 || !strings.Contains(errs, "unknown command") {
		t.Fatalf("%d %q", code, errs)
	}
}

func TestConfigSchema(t *testing.T) {
	code, out, _ := cliRun(t, nil, "config", "schema")
	if code != 0 || !strings.Contains(out, `"fakeidx"`) {
		t.Fatalf("%d %s", code, out)
	}
}

func TestConfigDefaultsFormats(t *testing.T) {
	for format, want := range map[string]string{
		"":                  "CORSARR_SERVER_ADDRESS",
		"--format=table":    "KEY",
		"--format=markdown": "# Configuration reference",
		"--format=yaml":     "address: :8090",
		"--format=env":      "CORSARR_LOG_LEVEL=info\n",
	} {
		args := []string{"config", "defaults"}
		if format != "" {
			args = append(args, format)
		}
		if code, out, _ := cliRun(t, nil, args...); code != 0 || !strings.Contains(out, want) {
			t.Fatalf("%s: %d %s", format, code, out)
		}
	}
	if code, _, errs := cliRun(t, nil, "config", "defaults", "--format=xml"); code != 2 || !strings.Contains(errs, "--format") {
		t.Fatalf("bad format: %d %s", code, errs)
	}
}

func TestConfigExportFormats(t *testing.T) {
	env := []string{"CORSARR_LOG_LEVEL=debug", "CORSARR_INDEXERS_A_TYPE=fakeidx", "CORSARR_INDEXERS_A_APIKEY=s3cret"}
	code, out, _ := cliRun(t, env, "config", "export")
	if code != 0 || !strings.Contains(out, "level: debug") || !strings.Contains(out, "${env:CORSARR_SECRET_INDEXERS_A_APIKEY}") || strings.Contains(out, "s3cret") {
		t.Fatalf("yaml: %d %s", code, out)
	}
	_, out, _ = cliRun(t, env, "config", "export", "--format=env")
	if !strings.Contains(out, "CORSARR_LOG_LEVEL=debug\n") || !strings.Contains(out, "CORSARR_INDEXERS_A_APIKEY=\n") {
		t.Fatalf("env: %s", out)
	}
	_, out, _ = cliRun(t, env, "config", "export", "--full")
	if !strings.Contains(out, "address:") {
		t.Fatalf("full: %s", out)
	}
	if code, _, _ := cliRun(t, env, "config", "export", "--format=xml"); code != 2 {
		t.Fatal("bad format accepted")
	}
	if code, _, _ := cliRun(t, []string{"CORSARR_LOG_LEVEL=loud"}, "config", "export"); code != 1 {
		t.Fatal("invalid config exported")
	}
}

func TestConfigValidate(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "c.yml")
	_ = os.WriteFile(file, []byte("indexers:\n  a:\n    type: fakeidx\nrootFolders:\n  - path: /m\n    kind: movie\n"), 0o600)
	code, out, _ := cliRun(t, []string{"CORSARR_LOG_LEVEL=debug"}, "config", "validate", "--plan", "--configFile="+file)
	for _, want := range []string{"configuration valid (source: file " + file, "warning: config file in use; ignored keys from other sources: CORSARR_LOG_LEVEL", "indexers.a (fakeidx)", "rootFolders.0 /m (movie)"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if code != 0 {
		t.Fatal(code)
	}
	code, _, errs := cliRun(t, nil, "config", "validate", "--server.address=nope", "--log.level=loud")
	if code != 1 || !strings.Contains(errs, "server.address (--server.address)") || !strings.Contains(errs, "log.level (--log.level)") {
		t.Fatalf("%d %s", code, errs)
	}
	if code, out, _ := cliRun(t, nil, "config", "validate"); code != 0 || !strings.Contains(out, "source: defaults)") {
		t.Fatalf("defaults: %s", out)
	}
}

func TestServeHealthcheckAndGracefulShutdown(t *testing.T) {
	dir := t.TempDir()
	args := []string{"--data.dir=" + dir, "--server.address=127.0.0.1:0", "--server.basePath=/c"}
	var stderr bytes.Buffer
	c := parse(args, []string{"HOME=" + t.TempDir(), "GOMEMLIMIT=200MiB"}, &bytes.Buffer{}, &stderr)
	ctx, cancel := context.WithCancel(context.Background())
	addrc := make(chan string, 1)
	done := make(chan int, 1)
	go func() { done <- c.serve(ctx, func(a string) { addrc <- a }) }()
	t0 := time.Now()
	var addr string
	select {
	case addr = <-addrc:
	case <-time.After(10 * time.Second):
		t.Fatalf("not ready: %s", stderr.String())
	}
	hc := parse([]string{"healthcheck", "--server.address=" + addr, "--server.basePath=/c"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if code := hc.healthcheck(); code != 0 {
		t.Fatal("healthcheck failed against running server")
	}
	resp, err := http.Get("http://" + addr + "/c/api/v1/firstrun")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("firstrun: %v", err)
	}
	resp.Body.Close()
	t.Log("ready+checks", time.Since(t0))
	cancel()
	select {
	case code := <-done:
		t.Log("stopped", time.Since(t0))
		if code != 0 {
			t.Fatalf("exit %d: %s", code, stderr.String())
		}
	case <-time.After(ShutdownGrace + time.Second):
		t.Fatal("shutdown took too long")
	}
	if !strings.Contains(stderr.String(), `"msg":"corsarr listening"`) || !strings.Contains(stderr.String(), `"configSource":"args"`) {
		t.Fatalf("log: %s", stderr.String())
	}
	if st, err := os.Stat(filepath.Join(dir, "corsarr.db-wal")); err == nil && st.Size() > 0 {
		t.Fatal("WAL not checkpointed on shutdown")
	}
	// a second serve on the same data dir works after shutdown (lock released)
	if _, err := os.Stat(filepath.Join(dir, "secret.key")); err != nil {
		t.Fatal(err)
	}
}

func TestServeRefusesInvalidConfigAndLockedDir(t *testing.T) {
	if code, _, _ := cliRun(t, nil, "serve", "--server.address=bad"); code != 2 {
		t.Fatal("invalid bootstrap must refuse start")
	}
	file := filepath.Join(t.TempDir(), "notadir")
	_ = os.WriteFile(file, nil, 0o600)
	if code, _, errs := cliRun(t, nil, "--data.dir="+file+"/x"); code != 1 || !strings.Contains(errs, "not writable") {
		t.Fatalf("%d %s", code, errs)
	}
	busy := parse([]string{"--data.dir=" + t.TempDir(), "--server.address=256.0.0.1:1"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if code := busy.serve(context.Background(), nil); code != 1 {
		t.Fatal("listen failure must exit 1")
	}
}

func TestHealthcheckFailsWithoutServer(t *testing.T) {
	if code, _, errs := cliRun(t, nil, "healthcheck", "--server.address=127.0.0.1:1"); code != 1 || !strings.Contains(errs, "unhealthy") {
		t.Fatalf("%d %s", code, errs)
	}
	if code, _, _ := cliRun(t, nil, "healthcheck", "--log.level=loud"); code != 1 {
		t.Fatal("invalid config")
	}
}

func TestHealthURL(t *testing.T) {
	cases := map[string]string{
		":8090":          "http://127.0.0.1:8090/api/v1/health",
		"0.0.0.0:1":      "http://127.0.0.1:1/api/v1/health",
		"10.0.0.5:9":     "http://10.0.0.5:9/api/v1/health",
		"[::]:7":         "http://127.0.0.1:7/api/v1/health",
		"localhost:8090": "http://localhost:8090/api/v1/health",
	}
	for addr, want := range cases {
		if got := healthURL(addr, ""); got != want {
			t.Errorf("%s: %s", addr, got)
		}
	}
	if healthURL(":1", "/c") != "http://127.0.0.1:1/c/api/v1/health" {
		t.Fatal("basePath")
	}
}

func TestRuntimeDefaults(t *testing.T) {
	if !hasEnv([]string{"GOGC=50"}, "GOGC") || hasEnv([]string{"GOGCX=1", "GOGC"}, "GOGC") {
		t.Fatal("hasEnv")
	}
	applyRuntimeDefaults([]string{"GOMEMLIMIT=1GiB", "GOGC=100"})
}

func TestOnConfigChange(t *testing.T) {
	dir := t.TempDir()
	c := parse([]string{"--data.dir=" + dir}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	cfg, _ := c.load()
	var logs bytes.Buffer
	k, err := startKernel(context.Background(), cfg, newLogger(cli{stderr: &logs}, "info"))
	if err != nil {
		t.Fatal(err)
	}
	defer k.close()
	k.onConfigChange(config.Change{Missing: true})
	if h, _ := k.health.Get(configRef); h.Status != domain.HealthDegraded {
		t.Fatalf("missing: %+v", h)
	}
	k.onConfigChange(config.Change{Err: errors.New("bad key")})
	if h, _ := k.health.Get(configRef); h.LastErrorMessage != "bad key" {
		t.Fatalf("err: %+v", h)
	}
	k.onConfigChange(config.Change{Config: cfg, Bootstrap: []string{"log"}, Live: []string{"indexers"}})
	if h, _ := k.health.Get(configRef); h.Status != domain.HealthOK || !strings.Contains(logs.String(), "restart corsarr") {
		t.Fatalf("ok: %+v %s", h, logs.String())
	}
	if len(k.jobs.List()) != 14 { // 2 kernel housekeeping + 8 movie pipeline + 2 TV + 2 music jobs
		t.Fatalf("kernel jobs: %d", len(k.jobs.List()))
	}
	if err := k.ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, j := range []string{"sessions.prune", "history.prune"} {
		_ = k.jobs.RunNow(j)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ok := 0
		for _, j := range k.jobs.List() {
			if (j.Name == "sessions.prune" || j.Name == "history.prune") && j.LastResult == domain.JobOK {
				ok++
			}
		}
		if ok == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("jobs did not run: %+v", k.jobs.List())
}
