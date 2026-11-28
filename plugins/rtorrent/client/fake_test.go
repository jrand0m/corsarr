package client

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
)

// infoDict is the bencoded info dictionary of the fixture torrent.
var infoDict = "d6:lengthi5e4:name1:x12:piece lengthi16384e6:pieces20:" + strings.Repeat("a", 20) + "e"

var torrentFile = []byte("d8:announce3:foo4:info" + infoDict + "e")

func fixtureHash() string {
	sum := sha1.Sum([]byte(infoDict))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

const (
	magnetHex = "0123456789abcdef0123456789abcdef01234567"
	magnet    = "magnet:?xt=urn:btih:" + magnetHex + "&dn=Some.Show"
)

// httpDoer adapts net/http to the Doer interface.
type httpDoer struct{ c *http.Client }

func (h httpDoer) Do(ctx context.Context, method, rawURL string, header map[string]string, body []byte) (int, map[string]string, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := h.c.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	out := map[string]string{}
	for k := range resp.Header {
		out[k] = resp.Header.Get(k)
	}
	return resp.StatusCode, out, b, err
}

// profile is the method set of one rTorrent build as recorded.
type profile struct {
	version, library string
	missing          map[string]bool // methods this build answers "not defined"
	erasedata        bool            // a web front end installed its data-erasing handler
}

var (
	// rTorrent 0.9.8: d.multicall2 and system.multicall; announce without .force.
	v098 = profile{version: "0.9.8", library: "0.13.8", erasedata: true,
		missing: map[string]bool{"d.tracker_announce.force": true}}
	// rTorrent 0.10: the same core methods; the forced announce exists.
	v010 = profile{version: "0.10.0", library: "0.14.0", erasedata: true, missing: map[string]bool{}}
	// An older build: no d.multicall2, no system.multicall, no manual
	// announce, no front end installed.
	legacy = profile{version: "0.9.2", library: "0.13.2",
		missing: map[string]bool{"d.multicall2": true, "system.multicall": true,
			"d.tracker_announce.force": true, "d.tracker_announce": true}}
)

type fakeTorrent struct {
	hash, name, dir, msg                       string
	custom                                     map[string]string
	started, active, open, complete, checking  bool
	multi                                      bool
	size, done, down, up, prio, ratio, seeders int64
	trackers                                   []string
	erasedData                                 bool
}

// fakeRT is a stateful in-memory rTorrent XML-RPC endpoint.
type fakeRT struct {
	srv        *httptest.Server
	prof       profile
	user, pass string

	mu        sync.Mutex
	torrents  map[string]*fakeTorrent
	calls     []string
	loads     [][]any // params of every load.* call
	downLimit int64
	upLimit   int64
	defDir    string
	raw       map[string][]byte // method → canned response body
	status    int               // non-zero: answer every request with this status
}

func newFake(t *testing.T, p profile) *fakeRT {
	t.Helper()
	f := &fakeRT{prof: p, user: "rt", pass: "pw", torrents: map[string]*fakeTorrent{}, defDir: "/downloads",
		raw: map[string][]byte{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeRT) client(t *testing.T, mod ...func(*Config)) *Client {
	t.Helper()
	cfg := Config{URL: f.srv.URL + "/RPC2", Username: f.user, Password: f.pass}
	for _, m := range mod {
		m(&cfg)
	}
	c, err := New(httpDoer{f.srv.Client()}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *fakeRT) called(method string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c == method {
			return true
		}
	}
	return false
}

func (f *fakeRT) add(t *fakeTorrent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t.custom == nil {
		t.custom = map[string]string{}
	}
	f.torrents[t.hash] = t
}

func (f *fakeRT) get(hash string) *fakeTorrent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.torrents[hash]
}

func (f *fakeRT) serve(w http.ResponseWriter, r *http.Request) {
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	if u, p, ok := r.BasicAuth(); !ok || u != f.user || p != f.pass {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(r.Body)
	method, params, err := DecodeCall(body, 0)
	if err != nil {
		_, _ = w.Write(EncodeFault(-32700, "parse error"))
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if canned, ok := f.raw[method]; ok {
		_, _ = w.Write(canned)
		return
	}
	v, fault := f.dispatch(method, params)
	if fault != nil {
		_, _ = w.Write(EncodeFault(fault.Code, fault.Message))
		return
	}
	out, err := EncodeResponse(v)
	if err != nil {
		panic(err)
	}
	_, _ = w.Write(out)
}

func noMethod(m string) *Fault { return &Fault{Code: -506, Message: "Method '" + m + "' not defined"} }

var noHash = &Fault{Code: -501, Message: "Could not find info-hash."}

type handler func(f *fakeRT, params []any) (any, *Fault)

var handlers map[string]handler

func init() {
	handlers = map[string]handler{
		"system.client_version":         func(f *fakeRT, _ []any) (any, *Fault) { return f.prof.version, nil },
		"system.library_version":        func(f *fakeRT, _ []any) (any, *Fault) { return f.prof.library, nil },
		"system.multicall":              (*fakeRT).multicallSystem,
		"directory.default":             func(f *fakeRT, _ []any) (any, *Fault) { return f.defDir, nil },
		"method.get":                    (*fakeRT).methodGet,
		"d.multicall2":                  func(f *fakeRT, p []any) (any, *Fault) { return f.multicall(p[1:]) },
		"d.multicall":                   func(f *fakeRT, p []any) (any, *Fault) { return f.multicall(p) },
		"t.multicall":                   (*fakeRT).trackerMulticall,
		"f.multicall":                   (*fakeRT).fileMulticall,
		"load.start":                    loader(true, false),
		"load.normal":                   loader(false, false),
		"load.raw_start":                loader(true, true),
		"load.raw":                      loader(false, true),
		"throttle.global_down.max_rate": func(f *fakeRT, _ []any) (any, *Fault) { return f.downLimit, nil },
		"throttle.global_up.max_rate":   func(f *fakeRT, _ []any) (any, *Fault) { return f.upLimit, nil },
		"throttle.global_down.max_rate.set": func(f *fakeRT, p []any) (any, *Fault) {
			f.downLimit = p[1].(int64)
			return int64(0), nil
		},
		"throttle.global_up.max_rate.set": func(f *fakeRT, p []any) (any, *Fault) {
			f.upLimit = p[1].(int64)
			return int64(0), nil
		},
		"throttle.global_down.rate": func(f *fakeRT, _ []any) (any, *Fault) { return int64(2048), nil },
		"throttle.global_up.rate":   func(f *fakeRT, _ []any) (any, *Fault) { return int64(512), nil },
	}
}

func (f *fakeRT) dispatch(method string, params []any) (any, *Fault) {
	f.calls = append(f.calls, method)
	if f.prof.missing[method] {
		return nil, noMethod(method)
	}
	if h, ok := handlers[method]; ok {
		return h(f, params)
	}
	if strings.HasPrefix(method, "d.") && len(params) > 0 {
		t, ok := f.torrents[strings.ToUpper(params[0].(string))]
		if !ok {
			return nil, noHash
		}
		return f.command(t, strings.TrimPrefix(method, "d."), params[1:])
	}
	return nil, noMethod(method)
}

func (f *fakeRT) multicallSystem(params []any) (any, *Fault) {
	out := []any{}
	for _, c := range params[0].([]any) {
		m := c.(map[string]any)
		v, fault := f.dispatch(m["methodName"].(string), m["params"].([]any))
		if fault != nil {
			out = append(out, map[string]any{"faultCode": int64(fault.Code), "faultString": fault.Message})
			continue
		}
		out = append(out, []any{v})
	}
	return out, nil
}

func (f *fakeRT) methodGet(params []any) (any, *Fault) {
	if params[1] != "event.download.erased" {
		return nil, &Fault{Code: -503, Message: "Key not found."}
	}
	m := map[string]any{"!_download_list": "ui.focus.erase="}
	if f.prof.erasedata {
		m["erasedata"] = "branch=d.custom5=,..."
	}
	return m, nil
}

func (f *fakeRT) sortedHashes(view string) []string {
	var out []string
	for h, t := range f.torrents {
		if view == "main" || (view == "started" && t.started) {
			out = append(out, h)
		}
	}
	sort.Strings(out)
	return out
}

func (f *fakeRT) multicall(params []any) (any, *Fault) {
	view := params[0].(string)
	rows := []any{}
	for _, h := range f.sortedHashes(view) {
		var row []any
		for _, c := range params[1:] {
			v, fault := f.command(f.torrents[h], strings.TrimSuffix(strings.TrimPrefix(c.(string), "d."), "="), nil)
			if fault != nil {
				return nil, fault
			}
			row = append(row, v)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// command runs one d.* command on a torrent.
func (f *fakeRT) command(t *fakeTorrent, cmd string, args []any) (any, *Fault) {
	if v, ok := f.getter(t, cmd); ok {
		return v, nil
	}
	if strings.HasPrefix(cmd, "custom") && strings.HasSuffix(cmd, ".set") {
		t.custom[strings.TrimSuffix(cmd, ".set")] = args[0].(string)
		return int64(0), nil
	}
	if strings.HasPrefix(cmd, "custom") {
		return t.custom[cmd], nil
	}
	return f.action(t, cmd, args)
}

func (f *fakeRT) getter(t *fakeTorrent, cmd string) (any, bool) {
	v, ok := map[string]any{
		"hash": t.hash, "name": t.name, "state": b2i(t.started), "is_active": b2i(t.active), "is_open": b2i(t.open),
		"complete": b2i(t.complete), "is_hash_checking": b2i(t.checking), "message": t.msg, "size_bytes": t.size,
		"completed_bytes": t.done, "left_bytes": t.size - t.done, "down.rate": t.down, "up.rate": t.up,
		"ratio": t.ratio, "peers_complete": t.seeders, "peers_accounted": t.seeders * 2, "priority": t.prio,
		"is_multi_file": b2i(t.multi), "directory": t.dir, "free_diskspace": int64(1 << 40),
	}[cmd]
	return v, ok
}

func (f *fakeRT) action(t *fakeTorrent, cmd string, args []any) (any, *Fault) {
	switch cmd {
	case "start":
		t.started, t.active, t.open = true, true, true
	case "stop":
		t.started, t.active = false, false
	case "pause":
		t.active = false
	case "resume":
		t.active = t.started
	case "erase":
		t.erasedData = t.custom["custom5"] == "1" && f.prof.erasedata
		delete(f.torrents, t.hash)
	case "priority.set":
		t.prio = args[0].(int64)
	case "update_priorities", "check_hash", "tracker_announce", "tracker_announce.force":
	default:
		return nil, noMethod("d." + cmd)
	}
	return int64(0), nil
}

func loader(start, raw bool) handler {
	return func(f *fakeRT, params []any) (any, *Fault) {
		f.loads = append(f.loads, params)
		if params[0] != "" {
			return nil, &Fault{Code: -501, Message: "Unsupported target type found."}
		}
		hash, name, ok := loadedHash(params[1], raw)
		if !ok {
			return nil, &Fault{Code: -503, Message: "Could not create download."}
		}
		t := &fakeTorrent{hash: hash, name: name, dir: f.defDir, size: 5, prio: 2, custom: map[string]string{},
			trackers: []string{"https://tracker.example/announce/SECRETPASSKEY?x=1", "dht://"}}
		for _, c := range params[2:] {
			applyLoadCommand(t, c.(string))
		}
		t.started, t.active, t.open = start, start, start
		f.torrents[hash] = t
		return int64(0), nil
	}
}

func loadedHash(payload any, raw bool) (hash, name string, ok bool) {
	if raw {
		b, isBytes := payload.([]byte)
		h, err := TorrentHash(b)
		return h, "x", isBytes && err == nil
	}
	s, _ := payload.(string)
	h, err := MagnetHash(s)
	u, _ := url.Parse(s)
	return h, u.Query().Get("dn"), err == nil
}

func applyLoadCommand(t *fakeTorrent, c string) {
	key, val, _ := strings.Cut(c, "=")
	val = strings.TrimSuffix(strings.TrimPrefix(val, `"`), `"`)
	val = strings.ReplaceAll(strings.ReplaceAll(val, `\"`, `"`), `\\`, `\`)
	switch {
	case key == "d.directory.set":
		t.dir = val
	case strings.HasPrefix(key, "d.custom"):
		t.custom[strings.TrimSuffix(strings.TrimPrefix(key, "d."), ".set")] = val
	}
}

func (f *fakeRT) trackerMulticall(params []any) (any, *Fault) {
	t, ok := f.torrents[params[0].(string)]
	if !ok {
		return nil, noHash
	}
	rows := []any{}
	for i, u := range t.trackers {
		rows = append(rows, []any{u, int64(1), int64(10), int64(3), int64(1 - i%2), int64(i % 2)})
	}
	return rows, nil
}

func (f *fakeRT) fileMulticall(params []any) (any, *Fault) {
	if _, ok := f.torrents[params[0].(string)]; !ok {
		return nil, noHash
	}
	return []any{
		[]any{"Show/e01.mkv", int64(1000), int64(5), int64(10), int64(1)},
		[]any{"Show/sample.mkv", int64(10), int64(0), int64(0), int64(0)},
	}, nil
}
