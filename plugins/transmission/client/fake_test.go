package client

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // the fake derives stand-in info hashes
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// httpDoer binds the Doer interface to net/http for tests.
type httpDoer struct{ c *http.Client }

func (d httpDoer) Do(ctx context.Context, method, url string, header map[string]string, body []byte) (int, map[string]string, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := d.c.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	h := map[string]string{}
	for k := range resp.Header {
		h[k] = resp.Header.Get(k)
	}
	return resp.StatusCode, h, b, err
}

// fakeTorrent is one torrent held by the fake server.
type fakeTorrent struct {
	hash, name, dir string
	labels          []string
	status, bp      int
	errCode         int
	errStr          string
	percent         float64
	size, left      int64
	rateDown        int64
	seeders         int
	files           []map[string]any
	trackers        []map[string]any
}

// fields renders the torrent as torrent-get reports it on a server of the
// given RPC version: labels exist from 16, percentComplete from 17.
func (t *fakeTorrent) fields(rpc, queuePos int) map[string]any {
	m := map[string]any{
		"id": queuePos + 1, "hashString": t.hash, "name": t.name, "status": t.status, "error": t.errCode,
		"errorString": t.errStr, "percentDone": t.percent, "sizeWhenDone": t.size, "leftUntilDone": t.left,
		"eta": -1, "rateDownload": t.rateDown, "rateUpload": 0, "uploadRatio": -1,
		"peersSendingToUs": t.seeders, "peersGettingFromUs": 0, "downloadDir": t.dir,
		"activityDate": 1700000000, "queuePosition": queuePos, "bandwidthPriority": t.bp,
		"files": t.files, "trackerStats": t.trackers,
	}
	if rpc >= 16 {
		m["labels"] = t.labels
	}
	if rpc >= 17 {
		m["percentComplete"] = t.percent
	}
	return m
}

// fakeTransmission is a recorded, stateful Transmission RPC server.
type fakeTransmission struct {
	t   *testing.T
	srv *httptest.Server

	mu          sync.Mutex
	version     string
	rpc         int
	user, pass  string
	sessionID   string
	handshakes  int
	alwaysStale bool
	torrents    []*fakeTorrent
	session     map[string]any
	calls       []string
	lastArgs    map[string]map[string]any
	results     map[string]string // method -> result string to answer with
	stats       map[string]any
}

// fakeVersions are the releases the fake imitates.
var fakeVersions = []struct {
	name, version string
	rpc           int
}{
	{"3.00", "3.00 (bb6b5a062e)", 16},
	{"4.0.6", "4.0.6 (38c164933e)", 17},
}

func newFake(t *testing.T, version string, rpc int) *fakeTransmission {
	t.Helper()
	f := &fakeTransmission{t: t, version: version, rpc: rpc, sessionID: "sess-1",
		lastArgs: map[string]map[string]any{}, results: map[string]string{},
		session: map[string]any{
			"download-dir": "/downloads", "speed-limit-down": 1000, "speed-limit-down-enabled": true,
			"speed-limit-up": 50, "speed-limit-up-enabled": false, "alt-speed-enabled": false,
			"units": map[string]any{"speed-bytes": 1000},
		}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTransmission) client(t *testing.T, cfg Config) *Client {
	t.Helper()
	cfg.URL = f.srv.URL
	if cfg.Username == "" && f.user != "" {
		cfg.Username, cfg.Password = f.user, f.pass
	}
	c, err := New(httpDoer{f.srv.Client()}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *fakeTransmission) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method != http.MethodPost || r.URL.Path != rpcPath {
		http.NotFound(w, r)
		return
	}
	if u, p, _ := r.BasicAuth(); f.user != "" && (u != f.user || p != f.pass) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if f.alwaysStale || r.Header.Get(sessionHeader) != f.sessionID {
		f.handshakes++
		w.Header().Set(sessionHeader, f.sessionID)
		w.WriteHeader(http.StatusConflict)
		return
	}
	var req struct {
		Method    string         `json:"method"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		f.t.Errorf("fake: bad request body: %v", err)
		return
	}
	f.calls = append(f.calls, req.Method)
	f.lastArgs[req.Method] = req.Arguments
	args, result := f.dispatch(req.Method, req.Arguments)
	if forced, ok := f.results[req.Method]; ok {
		args, result = nil, forced
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "arguments": args})
}

func (f *fakeTransmission) dispatch(method string, a map[string]any) (map[string]any, string) {
	switch method {
	case "session-get":
		return f.sessionGet(), "success"
	case "session-set":
		for k, v := range a {
			f.session[k] = v
		}
		return nil, "success"
	case "session-stats":
		return f.sessionStats(), "success"
	case "free-space":
		return f.freeSpace(a), "success"
	case "torrent-get":
		return f.torrentGet(a), "success"
	case "torrent-add":
		return f.torrentAdd(a)
	}
	return nil, f.torrentAction(method, a)
}

func (f *fakeTransmission) sessionGet() map[string]any {
	out := map[string]any{"version": f.version, "rpc-version": f.rpc}
	for k, v := range f.session {
		out[k] = v
	}
	if f.rpc >= 17 {
		out["rpc-version-semver"] = "5.3.0"
		out["session-id"] = f.sessionID
	}
	return out
}

func (f *fakeTransmission) sessionStats() map[string]any {
	if f.stats != nil {
		return f.stats
	}
	paused := 0
	for _, t := range f.torrents {
		if t.status == statusStopped {
			paused++
		}
	}
	return map[string]any{"downloadSpeed": 2048, "uploadSpeed": 512, "torrentCount": len(f.torrents),
		"activeTorrentCount": len(f.torrents) - paused, "pausedTorrentCount": paused}
}

func (f *fakeTransmission) freeSpace(a map[string]any) map[string]any {
	out := map[string]any{"path": a["path"], "size-bytes": int64(500) << 30}
	if f.rpc >= 17 {
		out["total_size"] = int64(1000) << 30
	}
	return out
}

func (f *fakeTransmission) selected(a map[string]any) []int {
	raw, ok := a["ids"].([]any)
	var out []int
	for i, t := range f.torrents {
		if !ok || containsID(raw, t.hash) {
			out = append(out, i)
		}
	}
	return out
}

func containsID(ids []any, hash string) bool {
	for _, id := range ids {
		if s, ok := id.(string); ok && strings.EqualFold(s, hash) {
			return true
		}
	}
	return false
}

func (f *fakeTransmission) torrentGet(a map[string]any) map[string]any {
	fields, _ := a["fields"].([]any)
	list := []map[string]any{}
	for _, i := range f.selected(a) {
		all := f.torrents[i].fields(f.rpc, i)
		row := map[string]any{}
		for _, name := range fields {
			if v, ok := all[name.(string)]; ok {
				row[name.(string)] = v
			}
		}
		list = append(list, row)
	}
	return map[string]any{"torrents": list}
}

func (f *fakeTransmission) torrentAdd(a map[string]any) (map[string]any, string) {
	hash, name, ok := payloadHash(a)
	if !ok {
		return nil, "invalid or corrupt torrent file"
	}
	for _, t := range f.torrents {
		if t.hash == hash {
			return map[string]any{"torrent-duplicate": map[string]any{"hashString": hash, "name": t.name, "id": 1}}, "success"
		}
	}
	t := &fakeTorrent{hash: hash, name: name, dir: str(a["download-dir"]), status: statusDownload, size: 1000, left: 1000,
		files: []map[string]any{{"name": name + "/" + name + ".mkv", "length": 1000, "bytesCompleted": 0}}}
	if t.dir == "" {
		t.dir = f.session["download-dir"].(string)
	}
	if p, _ := a["paused"].(bool); p {
		t.status = statusStopped
	}
	if labels, ok := a["labels"].([]any); ok && f.rpc >= 17 {
		t.labels = toStrings(labels)
	}
	f.torrents = append(f.torrents, t)
	return map[string]any{"torrent-added": map[string]any{"hashString": hash, "name": name, "id": len(f.torrents)}}, "success"
}

// payloadHash derives the info hash the fake files a payload under.
func payloadHash(a map[string]any) (hash, name string, ok bool) {
	if meta, has := a["metainfo"].(string); has {
		raw, err := base64.StdEncoding.DecodeString(meta)
		if err != nil || len(raw) == 0 || raw[0] != 'd' {
			return "", "", false
		}
		return sum(raw), "from-file", true
	}
	u := str(a["filename"])
	if _, after, found := strings.Cut(u, "btih:"); found && len(after) >= 40 {
		return strings.ToLower(after[:40]), "from-magnet", true
	}
	return sum([]byte(u)), "from-url", u != ""
}

func sum(b []byte) string {
	s := sha1.Sum(b) //nolint:gosec // see import
	return hex.EncodeToString(s[:])
}

func toStrings(v []any) []string {
	out := make([]string, 0, len(v))
	for _, x := range v {
		out = append(out, x.(string))
	}
	return out
}

func (f *fakeTransmission) torrentAction(method string, a map[string]any) string {
	sel := f.selected(a)
	switch {
	case method == "torrent-remove":
		f.remove(sel)
	case strings.HasPrefix(method, "queue-move-"):
		f.queueMove(strings.TrimPrefix(method, "queue-move-"), sel)
	default:
		return f.each(method, a, sel)
	}
	return "success"
}

func (f *fakeTransmission) each(method string, a map[string]any, sel []int) string {
	for _, i := range sel {
		t := f.torrents[i]
		switch method {
		case "torrent-stop":
			t.status = statusStopped
		case "torrent-start", "torrent-start-now":
			t.status = statusDownload
		case "torrent-verify":
			t.status = statusCheckWait
		case "torrent-reannounce":
		case "torrent-set":
			f.set(t, a)
		default:
			return "method name not recognized"
		}
	}
	return "success"
}

func (f *fakeTransmission) set(t *fakeTorrent, a map[string]any) {
	if labels, ok := a["labels"].([]any); ok && f.rpc >= 16 {
		t.labels = toStrings(labels)
	}
	if bp, ok := a["bandwidthPriority"].(float64); ok {
		t.bp = int(bp)
	}
}

func (f *fakeTransmission) remove(sel []int) {
	keep := f.torrents[:0]
	for i, t := range f.torrents {
		if !containsInt(sel, i) {
			keep = append(keep, t)
		}
	}
	f.torrents = keep
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func (f *fakeTransmission) queueMove(dir string, sel []int) {
	if len(sel) != 1 {
		return
	}
	i := sel[0]
	j := map[string]int{"top": 0, "bottom": len(f.torrents) - 1, "up": max(i-1, 0), "down": min(i+1, len(f.torrents)-1)}[dir]
	t := f.torrents[i]
	f.torrents = append(f.torrents[:i], f.torrents[i+1:]...)
	f.torrents = append(f.torrents[:j], append([]*fakeTorrent{t}, f.torrents[j:]...)...)
}

func (f *fakeTransmission) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.calls...)
}

func (f *fakeTransmission) args(method string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastArgs[method]
}

func (f *fakeTransmission) add(ts ...*fakeTorrent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.torrents = append(f.torrents, ts...)
}

func (f *fakeTransmission) snapshot(i int) fakeTorrent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *f.torrents[i]
}

func (f *fakeTransmission) handshakeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.handshakes
}
