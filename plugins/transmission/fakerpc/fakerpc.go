// Package fakerpc is a stateful fake of the Transmission RPC endpoint for
// tests: the session handshake, optional basic auth, the session answers
// recorded from Transmission 3.00 and 4.0.6, and enough torrent bookkeeping
// for the downloader conformance suite (add, list, control, remove).
package fakerpc

import (
	"crypto/sha1" //nolint:gosec // stand-in info hashes for added payloads, not security
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
)

// Version is one Transmission release the fake imitates.
type Version struct {
	Name    string // release, for test names
	RPC     int    // rpc-version
	session string // recorded session-get answer
}

// The releases the fake imitates.
var (
	V300 = Version{Name: "3.00", RPC: 16, session: "testdata/session-3.00.json"}
	V406 = Version{Name: "4.0.6", RPC: 17, session: "testdata/session-4.0.6.json"}
)

// Versions lists every imitated release.
var Versions = []Version{V300, V406}

//go:embed testdata/*.json
var recorded embed.FS

// Path is where the fake serves RPC.
const Path = "/transmission/rpc"

const sessionHeader = "X-Transmission-Session-Id"

// Transmission torrent status codes.
const (
	StatusStopped   = 0
	StatusCheckWait = 1
	StatusDownload  = 4
	StatusSeed      = 6
)

// Torrent is one torrent the fake holds.
type Torrent struct {
	Hash, Name, Dir string
	Labels          []string
	Status          int
	Priority        int // bandwidthPriority
	Error           int
	ErrorString     string
	Size, Left      int64
	RateDown        int64
	Peers           int
}

// Options adjust the fake.
type Options struct {
	Username, Password string // basic auth when set
}

// Server is the fake endpoint.
type Server struct {
	v    Version
	opts Options

	mu       sync.Mutex
	session  map[string]any
	torrents []*Torrent
	calls    []string
}

// New returns a fake of release v.
func New(v Version, opts Options) *Server {
	raw, err := recorded.ReadFile(v.session)
	if err != nil {
		panic(err)
	}
	s := &Server{v: v, opts: opts}
	if err := json.Unmarshal(raw, &s.session); err != nil {
		panic(err)
	}
	return s
}

// Torrents returns a copy of the held torrents in queue order.
func (s *Server) Torrents() []Torrent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Torrent, len(s.torrents))
	for i, t := range s.torrents {
		out[i] = *t
	}
	return out
}

// Add puts torrents into the fake.
func (s *Server) Add(ts ...Torrent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range ts {
		t := ts[i]
		s.torrents = append(s.torrents, &t)
	}
}

// Calls returns the RPC methods called so far.
func (s *Server) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

// Handler serves the RPC endpoint.
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serve) }

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != Path {
		http.NotFound(w, r)
		return
	}
	if u, p, _ := r.BasicAuth(); s.opts.Username != "" && (u != s.opts.Username || p != s.opts.Password) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Header.Get(sessionHeader) != "fake-session" {
		w.Header().Set(sessionHeader, "fake-session")
		w.WriteHeader(http.StatusConflict)
		return
	}
	var req struct {
		Method    string         `json:"method"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.calls = append(s.calls, req.Method)
	args, result := s.dispatch(req.Method, req.Arguments)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "arguments": args})
}

func (s *Server) dispatch(method string, a map[string]any) (map[string]any, string) {
	switch method {
	case "session-get":
		return s.session, "success"
	case "session-set":
		for k, v := range a {
			s.session[k] = v
		}
		return map[string]any{}, "success"
	case "session-stats":
		return s.stats(), "success"
	case "free-space":
		return map[string]any{"path": a["path"], "size-bytes": int64(500) << 30, "total_size": int64(1000) << 30}, "success"
	case "torrent-get":
		return s.get(a), "success"
	case "torrent-add":
		return s.add(a)
	case "torrent-remove":
		s.remove(s.selected(a))
		return map[string]any{}, "success"
	}
	if dir, ok := strings.CutPrefix(method, "queue-move-"); ok {
		s.move(dir, s.selected(a))
		return map[string]any{}, "success"
	}
	return map[string]any{}, s.each(method, a)
}

func (s *Server) stats() map[string]any {
	paused := 0
	for _, t := range s.torrents {
		if t.Status == StatusStopped {
			paused++
		}
	}
	return map[string]any{"downloadSpeed": 2048, "uploadSpeed": 512, "torrentCount": len(s.torrents),
		"activeTorrentCount": len(s.torrents) - paused, "pausedTorrentCount": paused}
}

// selected returns the indexes the ids argument names; no ids means all.
func (s *Server) selected(a map[string]any) []int {
	raw, filtered := a["ids"].([]any)
	var out []int
	for i, t := range s.torrents {
		if !filtered || slices.ContainsFunc(raw, func(id any) bool { v, _ := id.(string); return strings.EqualFold(v, t.Hash) }) {
			out = append(out, i)
		}
	}
	return out
}

func (s *Server) fields(t *Torrent, pos int) map[string]any {
	percent := 0.0
	if t.Size > 0 {
		percent = float64(t.Size-t.Left) / float64(t.Size)
	}
	m := map[string]any{
		"id": pos + 1, "hashString": t.Hash, "name": t.Name, "status": t.Status, "error": t.Error,
		"errorString": t.ErrorString, "percentDone": percent, "sizeWhenDone": t.Size, "leftUntilDone": t.Left,
		"eta": -1, "rateDownload": t.RateDown, "rateUpload": 0, "uploadRatio": -1,
		"peersSendingToUs": t.Peers, "peersGettingFromUs": 0, "downloadDir": t.Dir,
		"activityDate": 1700000000, "queuePosition": pos, "bandwidthPriority": t.Priority,
		"files":        []map[string]any{{"name": t.Name + "/" + t.Name + ".mkv", "length": t.Size, "bytesCompleted": t.Size - t.Left}},
		"trackerStats": []map[string]any{{"announce": "https://tracker.example/announce/secret", "announceState": 0, "hasAnnounced": true, "lastAnnounceSucceeded": true, "seederCount": 3, "leecherCount": 1}},
	}
	if s.v.RPC >= 16 {
		m["labels"] = t.Labels
	}
	if s.v.RPC >= 17 {
		m["percentComplete"] = percent
	}
	return m
}

func (s *Server) get(a map[string]any) map[string]any {
	fields, _ := a["fields"].([]any)
	list := []map[string]any{}
	for _, i := range s.selected(a) {
		all := s.fields(s.torrents[i], i)
		row := map[string]any{}
		for _, f := range fields {
			name, _ := f.(string)
			if v, ok := all[name]; ok {
				row[name] = v
			}
		}
		list = append(list, row)
	}
	return map[string]any{"torrents": list}
}

func (s *Server) add(a map[string]any) (map[string]any, string) {
	hash, name, ok := payloadHash(a)
	if !ok {
		return nil, "invalid or corrupt torrent file"
	}
	for _, t := range s.torrents {
		if t.Hash == hash {
			return map[string]any{"torrent-duplicate": map[string]any{"hashString": hash, "name": t.Name, "id": 1}}, "success"
		}
	}
	t := &Torrent{Hash: hash, Name: name, Status: StatusDownload, Size: 1000, Left: 1000}
	t.Dir, _ = a["download-dir"].(string)
	if t.Dir == "" {
		t.Dir, _ = s.session["download-dir"].(string)
	}
	if p, _ := a["paused"].(bool); p {
		t.Status = StatusStopped
	}
	if labels, ok := a["labels"].([]any); ok && s.v.RPC >= 17 {
		t.Labels = strs(labels)
	}
	s.torrents = append(s.torrents, t)
	return map[string]any{"torrent-added": map[string]any{"hashString": hash, "name": name, "id": len(s.torrents)}}, "success"
}

// payloadHash is the info hash of a magnet, or a stand-in for file bytes and
// addresses.
func payloadHash(a map[string]any) (hash, name string, ok bool) {
	if meta, has := a["metainfo"].(string); has {
		raw, err := base64.StdEncoding.DecodeString(meta)
		if err != nil || len(raw) == 0 || raw[0] != 'd' {
			return "", "", false
		}
		return digest(raw), "from-file", true
	}
	u, _ := a["filename"].(string)
	if _, after, found := strings.Cut(u, "btih:"); found && len(after) >= 40 {
		return strings.ToLower(after[:40]), "from-magnet", true
	}
	return digest([]byte(u)), "from-url", u != ""
}

func digest(b []byte) string {
	sum := sha1.Sum(b) //nolint:gosec // see import
	return hex.EncodeToString(sum[:])
}

func strs(v []any) []string {
	out := make([]string, 0, len(v))
	for _, x := range v {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

var statusAfter = map[string]int{"torrent-stop": StatusStopped, "torrent-start": StatusDownload,
	"torrent-start-now": StatusDownload, "torrent-verify": StatusCheckWait}

func (s *Server) each(method string, a map[string]any) string {
	st, known := statusAfter[method]
	if !known && method != "torrent-set" && method != "torrent-reannounce" {
		return "method name not recognized"
	}
	for _, i := range s.selected(a) {
		t := s.torrents[i]
		switch {
		case known:
			t.Status = st
		case method == "torrent-set":
			s.set(t, a)
		}
	}
	return "success"
}

func (s *Server) set(t *Torrent, a map[string]any) {
	if labels, ok := a["labels"].([]any); ok && s.v.RPC >= 16 {
		t.Labels = strs(labels)
	}
	if bp, ok := a["bandwidthPriority"].(float64); ok {
		t.Priority = int(bp)
	}
}

func (s *Server) remove(sel []int) {
	keep := s.torrents[:0]
	for i, t := range s.torrents {
		if !slices.Contains(sel, i) {
			keep = append(keep, t)
		}
	}
	s.torrents = keep
}

func (s *Server) move(dir string, sel []int) {
	if len(sel) != 1 {
		return
	}
	i, last := sel[0], len(s.torrents)-1
	j := map[string]int{"top": 0, "bottom": last, "up": max(i-1, 0), "down": min(i+1, last)}[dir]
	t := s.torrents[i]
	s.torrents = slices.Delete(s.torrents, i, i+1)
	s.torrents = slices.Insert(s.torrents, j, t)
}
