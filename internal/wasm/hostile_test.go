package wasm

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/wasm/wasmtest"
	"github.com/jrand0m/corsarr/plugins/example-downloader/fakeapi"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

type hostile struct {
	m   *Module
	ev  *events
	log *logBuf
}

func loadHostile(t *testing.T, mutate ...func(*abi.Manifest)) hostile {
	t.Helper()
	ev, lb := &events{}, &logBuf{}
	e := engine(t, func(o *Options) {
		o.Events = ev
		o.Log = slog.New(slog.NewTextHandler(lb, nil))
	})
	return hostile{m: load(t, e, wasmtest.Hostile(t), wasmtest.HostileManifest(mutate...)), ev: ev, log: lb}
}

func (h hostile) attack(t *testing.T, attack string, extra ...string) error {
	t.Helper()
	s := map[string]any{"attack": attack, "other": "s3cret"}
	if len(extra) == 2 {
		s[extra[0]] = extra[1]
	}
	p, err := h.m.NewPool(Binding{Key: attack, Settings: s, HTTP: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Drain(context.Background())
	return NewHandle(p).Test(context.Background())
}

// Hostile behaviour fails only the offending call.
func TestHostileCalls(t *testing.T) {
	h := loadHostile(t)
	lan := httptest.NewServer(http.NotFoundHandler())
	defer lan.Close()
	cases := []struct {
		attack string
		extra  []string
		class  plugin.FailureClass
		msg    string
	}{
		{"host", nil, plugin.ClassConfig, "not declared in permissions.http.hosts"},
		{"declared", []string{"target", "http://private.example.org/"}, "", ""}, // the setting names it: allowed to resolve privately
		{"alloc", nil, plugin.ClassServerError, "failed inside the module"},
		{"loop", nil, plugin.ClassTimeout, "exceeded the 1.5s call limit"},
		{"sleep", nil, plugin.ClassTimeout, "exceeded the 1.5s call limit"},
		{"panic", nil, plugin.ClassServerError, "panic in test: handler bug"},
		{"crash", nil, plugin.ClassServerError, "failed inside the module"},
		{"secret", nil, plugin.ClassConfig, "not declared in permissions.secrets"},
		{"event", nil, plugin.ClassConfig, "not allowed for this kind"},
		{"nope", nil, plugin.ClassConfig, "unknown attack"},
	}
	for _, c := range cases {
		t.Run(c.attack, func(t *testing.T) {
			err := h.attack(t, c.attack, c.extra...)
			if c.class == "" {
				if err != nil && !strings.Contains(err.Error(), "unreachable") && !strings.Contains(err.Error(), "resolve") &&
					!strings.Contains(err.Error(), "dial") && !strings.Contains(err.Error(), "HTTP") {
					t.Errorf("unexpected refusal: %v", err)
				}
				return
			}
			if plugin.ClassOf(err) != c.class || !strings.Contains(err.Error(), c.msg) {
				t.Errorf("got %v (%q), want %s %q", err, plugin.ClassOf(err), c.class, c.msg)
			}
		})
	}
}

func TestHostilePrivateResolution(t *testing.T) {
	h := loadHostile(t)
	// declared public host that resolves to a private address
	p, _ := h.m.NewPool(Binding{Key: "x", Settings: map[string]any{"attack": "declared", "target": "http://ok.example.org/"}, HTTP: http.DefaultClient})
	defer p.Drain(context.Background())
	_, _, err := p.check(context.Background(), abi.HTTPRequest{Method: "GET", URL: "http://private.example.org/"})
	if err == nil || !strings.Contains(err.Error(), "private") {
		t.Errorf("private resolution allowed: %v", err)
	}
	if _, _, err := p.check(context.Background(), abi.HTTPRequest{Method: "GET", URL: "http://127.0.0.1:1/"}); err == nil {
		t.Error("undeclared loopback literal allowed")
	}
	for _, bad := range []abi.HTTPRequest{{Method: "GET", URL: "file:///etc/passwd"}, {Method: "GET", URL: "http://u:p@ok.example.org/"},
		{Method: "TRACE", URL: "http://ok.example.org/"}} {
		if _, _, err := p.check(context.Background(), bad); err == nil {
			t.Errorf("%+v allowed", bad)
		}
	}
	u, g, err := p.check(context.Background(), abi.HTTPRequest{Method: "GET", URL: "http://ok.example.org/x"})
	if err != nil || u.Host != "ok.example.org" || !g.allowPrivate {
		t.Errorf("configured host: %v %v %v", u, g, err)
	}
}

func TestHostileDoesNotAffectOthers(t *testing.T) {
	h := loadHostile(t)
	e := engine(t)
	good, _ := exampleClient(t, e)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = h.attack(t, "loop") }()
	start := time.Now()
	for range 5 {
		if err := good.Test(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("healthy instance slowed to %v while another loops", took)
	}
	wg.Wait()
}

func TestTrapRecyclesAndDegrades(t *testing.T) {
	h := loadHostile(t)
	settings := map[string]any{"attack": "crash"}
	p, _ := h.m.NewPool(Binding{Key: "d", Settings: settings, HTTP: http.DefaultClient})
	defer p.Drain(context.Background())
	hd := NewHandle(p)
	for range degradeAfter {
		_ = hd.Test(context.Background())
	}
	settings["attack"] = "none"
	if err := hd.Test(context.Background()); err == nil || !strings.Contains(err.Error(), "degraded") {
		t.Fatalf("not degraded: %v", err)
	}
	p.coolUntil = time.Time{}
	if err := hd.Test(context.Background()); err != nil {
		t.Fatalf("instance did not recover after a trap: %v", err)
	}
}

func TestHostileOutputs(t *testing.T) {
	h := loadHostile(t)
	for _, a := range []string{"spam", "stdout", "clock", "emit"} {
		if err := h.attack(t, a); err != nil {
			t.Errorf("%s: %v", a, err)
		}
	}
	logs := h.log.String()
	if n := strings.Count(logs, "msg=line"); n != maxLogLines {
		t.Errorf("%d log lines, want the cap %d", n, maxLogLines)
	}
	if !strings.Contains(logs, "hello from stdout") || !strings.Contains(logs, "plugin=hostile") {
		t.Errorf("stdout not logged: %s", logs[max(0, len(logs)-300):])
	}
	if len(h.ev.got) != 1 || h.ev.got[0].Actor != "plugin:hostile" || string(h.ev.got[0].Type) != "download.progress" {
		t.Errorf("events %+v", h.ev.got)
	}
}

func TestHostileResultTooLarge(t *testing.T) {
	e := engine(t, func(o *Options) { o.Limits.MaxMessageBytes = 1 << 20 })
	m := load(t, e, wasmtest.Hostile(t), wasmtest.HostileManifest(func(m *abi.Manifest) { m.Limits.CallTimeoutMs = 10000 }))
	p, _ := m.NewPool(Binding{Key: "big"})
	defer p.Drain(context.Background())
	d := kinds[abi.KindDownloader].Adapter(NewHandle(p)).(*Downloader)
	if _, err := d.Add(context.Background(), plugin.AddRequest{}); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("oversized answer: %v", err)
	}
	if !d.KeepsDataOnRemove() || !is[plugin.Rechecker](d) || is[plugin.FileLister](d) || !is[plugin.PriorityLeveler](d) {
		t.Error("capability gate")
	}
	if err := d.SetPriorityLevel(context.Background(), "x", plugin.PrioHigh); !errors.Is(err, plugin.ErrNotFound) {
		t.Errorf("not found: %v", err)
	}
	if err := d.Recheck(context.Background(), "x"); !errors.Is(err, plugin.ErrUnsupported) {
		t.Errorf("unsupported: %v", err)
	}
	if _, err := d.Stats(context.Background()); !errors.Is(err, plugin.ErrUnsupported) {
		t.Errorf("missing op: %v", err)
	}
}

func TestDrainWaitsForInflight(t *testing.T) {
	h := loadHostile(t)
	p, _ := h.m.NewPool(Binding{Key: "drain", Settings: map[string]any{"attack": "sleep"}})
	errc := make(chan error, 1)
	go func() { errc <- NewHandle(p).Test(context.Background()) }()
	time.Sleep(200 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p.Drain(ctx)
	if err := <-errc; err == nil {
		t.Error("in-flight call should end at its deadline")
	}
	if err := NewHandle(p).Test(context.Background()); !errors.Is(err, errUnloaded) {
		t.Errorf("call after drain: %v", err)
	}
}

func TestReplaceSwapsPool(t *testing.T) {
	e := engine(t)
	d, _ := exampleClient(t, e)
	h := d.ModuleHandle()
	old := h.Pool()
	m2 := load(t, e, wasmtest.Example(t), nil)
	srv := httptest.NewServer(fakeapi.New("").Handler())
	defer srv.Close()
	p2, _ := m2.NewPool(Binding{Key: "ex", Settings: map[string]any{"url": srv.URL}, HTTP: http.DefaultClient})
	if got, ok := h.Swap(p2); !ok || got != old {
		t.Fatal("swap returned the wrong pool")
	}
	old.Drain(context.Background())
	if err := d.Test(context.Background()); err != nil {
		t.Fatalf("after swap: %v", err)
	}
	if err := d.Close(); err != nil || d.Close() != nil {
		t.Fatal("close")
	}
	if _, ok := h.Swap(p2); ok {
		t.Error("swap into a closed instance")
	}
	if _, err := m2.NewPool(Binding{}); err != nil {
		t.Fatal(err)
	}
	_ = m2.Close(context.Background())
	if _, err := m2.NewPool(Binding{}); err == nil {
		t.Error("pool from a closed module")
	}
}
