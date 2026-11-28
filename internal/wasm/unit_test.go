package wasm

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/httpx"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/wasm/wasmtest"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

func TestLinkChecks(t *testing.T) {
	e := engine(t)
	man := wasmtest.HostileManifest()
	full := []string{abi.ExportMarker, abi.ExportInit}
	cases := map[string][]byte{
		"imports env.foo, which this corsarr does not provide": wasmtest.Assemble([]wasmtest.Import{{Module: "env", Name: "foo"}}, full),
		"imports corsarr_v1.exec":                              wasmtest.Assemble([]wasmtest.Import{{Module: abi.HostModule, Name: "exec"}}, full),
		"imports wasi_snapshot_preview1.sock_open":             wasmtest.Assemble([]wasmtest.Import{{Module: wasiModule, Name: "sock_open"}}, full),
		"imports corsarr_v1.log with the wrong signature":      wasmtest.Assemble([]wasmtest.Import{{Module: abi.HostModule, Name: abi.HostLog}}, full),
		"module speaks ABI v2, this corsarr supports v1":       wasmtest.Assemble(nil, []string{"corsarr_abi_v2"}),
		"not a corsarr plugin module":                          wasmtest.Assemble(nil, []string{"main"}),
		"built as a command; build it as a reactor":            wasmtest.Assemble(nil, []string{abi.ExportMarker, abi.ExportStart}),
		"missing the _initialize export":                       wasmtest.Assemble(nil, []string{abi.ExportMarker}),
		"missing or mistyped export corsarr_":                  wasmtest.Assemble(nil, []string{abi.ExportMarker, abi.ExportInit, abi.ExportAlloc}),
		"not a valid WebAssembly module":                       []byte("\x00asm\x01\x00\x00\x00\xff\xff"),
	}
	for want, module := range cases {
		_, err := e.Load(context.Background(), module, man)
		var le *LoadError
		if !errors.As(err, &le) || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", want, err)
		}
	}
}

func TestLoadRefusals(t *testing.T) {
	e := engine(t, func(o *Options) { o.Limits.MaxModuleBytes = 1 << 30 })
	if _, err := e.Load(context.Background(), wasmtest.Assemble(nil, nil), nil); !errors.Is(err, ErrNoManifest) {
		t.Errorf("no manifest: %v", err)
	}
	if _, err := e.Load(context.Background(), wasmtest.Assemble(nil, nil), []byte(`{"id":`)); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("bad json: %v", err)
	}
	bad := wasmtest.HostileManifest(func(m *abi.Manifest) { m.ABIVersion = 2; m.Settings.Fields[0].Label = "<x>\u202E" })
	_, err := e.Load(context.Background(), wasmtest.Hostile(t), bad)
	var me *ManifestError
	if !errors.As(err, &me) || !strings.Contains(err.Error(), "abiVersion") || !strings.Contains(err.Error(), "settings.fields[0].label") {
		t.Errorf("bad manifest: %v", err)
	}
	wrongKind := wasmtest.Embed(wasmtest.Hostile(t), wasmtest.HostileManifest())
	small := engine(t, func(o *Options) { o.Limits.MaxModuleBytes = 1000 })
	if _, err := small.Load(context.Background(), wrongKind, nil); err == nil || !strings.Contains(err.Error(), "the limit is 1000") {
		t.Errorf("size cap: %v", err)
	}
}

func TestManifestChecks(t *testing.T) {
	m := abi.Manifest{ID: "X-1", Version: "1.0", Kind: "indexer", Kinds: []string{"downloader", "indexer"}, ABIVersion: 1,
		Name: "", License: "not a licence!", Homepage: "http://x.org", Limits: abi.Limits{MemoryMiB: -1},
		Permissions: abi.Permissions{HTTP: abi.HTTPPermission{Hosts: []string{"*.com", "$setting:missing", "bad host", "api.example.org"}},
			Secrets: []string{"nope"}}}
	got := CheckManifest(m).Error()
	for _, want := range []string{"id: must be", "version: must be a semantic", "name: is required", "license: must be",
		"homepage: must be an https", "kinds: a module implements exactly one kind", "hosts[0]", "hosts[1]", "hosts[2]",
		"secrets[0]", "limits: must not be negative"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "hosts[3]") {
		t.Errorf("valid host flagged: %s", got)
	}
	if !strings.Contains(CheckManifest(abi.Manifest{Kind: "chatbot"}).Error(), `"chatbot" is not a kind`) {
		t.Error("unknown kind")
	}
	if ManifestKind(abi.Manifest{Kinds: []string{"downloader"}}) != "downloader" || ManifestKind(abi.Manifest{}) != "" {
		t.Error("ManifestKind")
	}
}

func TestCustomSection(t *testing.T) {
	mod := wasmtest.Assemble(nil, []string{"a"})
	if _, ok := CustomSection(mod, abi.ManifestSection); ok {
		t.Error("section in a bare module")
	}
	with := AppendCustomSection(AppendCustomSection(mod, "other", []byte("x")), abi.ManifestSection, []byte(`{"id":"a"}`))
	if b, ok := CustomSection(with, abi.ManifestSection); !ok || string(b) != `{"id":"a"}` {
		t.Errorf("%q %v", b, ok)
	}
	for _, broken := range [][]byte{[]byte("nope"), append(append([]byte{}, mod[:8]...), 0, 0xff), append(append([]byte{}, mod[:8]...), 0, 2, 9, 1)} {
		if _, ok := CustomSection(broken, "x"); ok {
			t.Errorf("parsed %x", broken)
		}
	}
	if b, err := ManifestBytes(mod, []byte("side")); err != nil || string(b) != "side" {
		t.Error("sidecar")
	}
}

func TestGuardedDial(t *testing.T) {
	var dialed string
	dial := func(_ context.Context, _, addr string) (net.Conn, error) { dialed = addr; return nil, nil }
	resolve := func(_ context.Context, host string) ([]netip.Addr, error) {
		if host == "fail" {
			return nil, errors.New("no such host")
		}
		if host == "rebind" {
			return []netip.Addr{netip.MustParseAddr("172.16.0.1")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("10.1.1.1"), netip.MustParseAddr("1.1.1.1")}, nil
	}
	d := guardedDial(dial, resolve)
	guarded := context.WithValue(context.Background(), guardKey{}, dialGuard{})
	if _, err := d(guarded, "tcp", "mixed:80"); err != nil || dialed != "1.1.1.1:80" {
		t.Errorf("public address not chosen: %s %v", dialed, err)
	}
	if _, err := d(guarded, "tcp", "rebind:80"); err == nil {
		t.Error("private-only name dialed")
	}
	if _, err := d(guarded, "tcp", "fail:80"); err == nil {
		t.Error("resolution failure ignored")
	}
	if _, err := d(guarded, "tcp", "noport"); err == nil {
		t.Error("bad address")
	}
	if _, err := d(context.Background(), "tcp", "rebind:80"); err != nil || dialed != "rebind:80" {
		t.Error("unguarded requests dial as usual")
	}
	allow := context.WithValue(context.Background(), guardKey{}, dialGuard{allowPrivate: true})
	if _, err := d(allow, "tcp", "rebind:81"); err != nil || dialed != "rebind:81" {
		t.Error("configured host dials as usual")
	}
	if GuardedTransport().DialContext == nil {
		t.Error("transport without dialer")
	}
}

func TestHostRules(t *testing.T) {
	r := newHostRules([]string{"API.example.org", "*.cdn.example", "$setting:url", "$setting:host"})
	conf := r.configuredHosts(map[string]any{"url": "http://Box.lan:9091/x", "host": "nas.local:8080"})
	for host, want := range map[string][2]bool{
		"api.example.org": {true, false}, "x.cdn.example": {true, false}, "cdn.example": {false, false},
		"box.lan": {true, true}, "nas.local": {true, true}, "other.org": {false, false},
	} {
		a, p := r.verdict(host, conf)
		if a != want[0] || p != want[1] {
			t.Errorf("%s: %v %v", host, a, p)
		}
	}
	for v, want := range map[any]string{"[::1]": "::1", "": "", 5: "", "10.0.0.1": "10.0.0.1"} {
		if got := settingHost(v); got != want {
			t.Errorf("%v: %q", v, got)
		}
	}
	for ip, want := range map[string]bool{"10.0.0.1": true, "127.0.0.1": true, "169.254.1.1": true, "100.64.0.1": true, "::1": true,
		"fe80::1": true, "8.8.8.8": false, "::ffff:172.16.0.1": true} {
		if restricted(netip.MustParseAddr(ip)) != want {
			t.Errorf("%s", ip)
		}
	}
}

func TestErrorsAndText(t *testing.T) {
	for class, want := range map[string]plugin.FailureClass{abi.ClassAuth: plugin.ClassAuth, abi.ClassDenied: plugin.ClassConfig, "weird": plugin.ClassServerError} {
		if got := plugin.ClassOf(moduleError(&abi.Error{Class: class, Message: "m"})); got != want {
			t.Errorf("%s: %s", class, got)
		}
	}
	if !errors.Is(moduleError(&abi.Error{Class: abi.ClassNotFound}), plugin.ErrNotFound) {
		t.Error("not found")
	}
	if got := CleanText("a\u202Eb\x00c"+strings.Repeat("x", 10), 5); got != "a b c…" {
		t.Errorf("%q", got)
	}
	if err := decodeEnvelope([]byte("nope"), nil); plugin.ClassOf(err) != plugin.ClassParse {
		t.Error(err)
	}
	var out struct{ A int }
	if err := decodeEnvelope([]byte(`{"result":"str"}`), &out); plugin.ClassOf(err) != plugin.ClassParse {
		t.Error(err)
	}
	if decodeEnvelope([]byte(`{}`), &out) != nil {
		t.Error("empty result")
	}
	var env abi.Envelope
	_ = json.Unmarshal(envelope(nil, httpx.ErrEgressBlocked{Host: "x"}), &env)
	if env.Error == nil || env.Error.Class != abi.ClassDenied {
		t.Errorf("egress block: %+v", env.Error)
	}
	_ = json.Unmarshal(envelope(nil, plugin.Errorf(plugin.ClassAuth, "x")), &env)
	if env.Error.Class != abi.ClassAuth {
		t.Errorf("classified: %+v", env.Error)
	}
	_ = json.Unmarshal(envelope(nil, context.DeadlineExceeded), &env)
	if env.Error.Class != abi.ClassTimeout {
		t.Errorf("timeout: %+v", env.Error)
	}
}

func TestLimits(t *testing.T) {
	l := DefaultLimits()
	c := l.clamp(abi.Limits{MemoryMiB: 1000, CallTimeoutMs: 500, Concurrency: 1})
	if c.MemoryMiB != 64 || c.CallTimeout != 500*time.Millisecond || c.Concurrency != 1 {
		t.Errorf("%+v", c)
	}
	if (Limits{}).withDefaults() != l {
		t.Error("defaults")
	}
	if KindNames()[0] != abi.KindDownloader {
		t.Error(KindNames())
	}
	if _, ok := Spec("nope"); ok {
		t.Error("spec")
	}
	if _, err := NewEngine(Options{CacheDir: "/dev/null/x"}); err == nil {
		t.Error("bad cache dir accepted")
	}
}

func TestStateOutsideCall(t *testing.T) {
	st := stateOf(context.Background())
	if st.now().IsZero() {
		t.Error("now")
	}
	st.output(1, []byte("partial"))
	st.output(1, []byte(" line\nnext"))
	if res, err := hostSettings(context.Background(), st, nil); err != nil || len(res.(abi.SettingsResponse).Values) != 0 {
		t.Error("settings without pool")
	}
	if _, err := hostSecret(context.Background(), st, []byte(`{"name":"x"}`)); err == nil {
		t.Error("secret without pool")
	}
	if _, err := hostEmit(context.Background(), st, []byte(`{"type":"x"}`)); err == nil {
		t.Error("emit without pool")
	}
	if _, err := hostEmit(context.Background(), st, make([]byte, maxEventBytes+1)); err == nil {
		t.Error("oversized event")
	}
	if _, err := hostHTTP(context.Background(), st, []byte(`{}`)); err == nil {
		t.Error("http without pool")
	}
	if _, err := hostLog(context.Background(), st, []byte(`nope`)); err == nil {
		t.Error("malformed log")
	}
	if _, err := hostLog(context.Background(), st, []byte(`{"level":"loud","message":"m","attrs":{"k":1}}`)); err != nil {
		t.Error(err)
	}
	if _, err := hostSleep(context.Background(), st, []byte(`{"millis":1}`)); err != nil {
		t.Error(err)
	}
	if _, err := hostNow(context.Background(), st, nil); err != nil {
		t.Error(err)
	}
	if _, ok := allowedImport("nope", "x"); ok {
		t.Error("import")
	}
}
