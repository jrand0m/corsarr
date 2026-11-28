// Package fakexmlrpc is a stateful fake of rTorrent's XML-RPC endpoint for
// tests, built on the client's strict codec. Profiles imitate a plain
// rTorrent 0.9.8 and rTorrent 0.10 behind a web front end (forced announce
// and the front end's data-erasing handler).
package fakexmlrpc

import (
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/jrand0m/corsarr/plugins/rtorrent/client"
)

// Profile is the method set of one rTorrent build as recorded.
type Profile struct {
	Name, Version, Library string
	Missing                []string // methods this build answers "not defined"
	EraseData              bool     // a front end installed its data-erasing handler
}

// The builds the fake imitates.
var (
	V098 = Profile{Name: "0.9.8", Version: "0.9.8", Library: "0.13.8", Missing: []string{"d.tracker_announce.force"}}
	V010 = Profile{Name: "0.10", Version: "0.10.0", Library: "0.14.0", EraseData: true}
)

// Profiles lists every imitated build.
var Profiles = []Profile{V098, V010}

// Options adjust the fake.
type Options struct {
	Username, Password string // basic auth when set
}

// Torrent is one download the fake holds.
type Torrent struct {
	Hash, Name, Dir, Message                  string
	Custom                                    map[string]string
	Started, Active, Open, Complete, Checking bool
	Size, Done, DownRate, Priority            int64
	ErasedData                                bool // removed with the data flag set (front end only)
}

// Server is the fake endpoint.
type Server struct {
	p    Profile
	opts Options

	mu        sync.Mutex
	torrents  map[string]*Torrent
	erased    []Torrent
	downLimit int64
	upLimit   int64
	free      int64
}

// New returns a fake of build p.
func New(p Profile, opts Options) *Server {
	return &Server{p: p, opts: opts, torrents: map[string]*Torrent{}, free: 1 << 40}
}

// SetFreeSpace sets the bytes free the fake reports for every download.
func (s *Server) SetFreeSpace(n int64) {
	s.mu.Lock()
	s.free = n
	s.mu.Unlock()
}

// Torrents returns copies of the held downloads, sorted by hash.
func (s *Server) Torrents() []Torrent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Torrent, 0, len(s.torrents))
	for _, h := range s.hashes("main") {
		out = append(out, *s.torrents[h])
	}
	return out
}

// Erased returns the downloads removed so far.
func (s *Server) Erased() []Torrent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.erased)
}

// Add puts downloads into the fake.
func (s *Server) Add(ts ...Torrent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range ts {
		t := ts[i]
		if t.Custom == nil {
			t.Custom = map[string]string{}
		}
		s.torrents[t.Hash] = &t
	}
}

// Handler serves XML-RPC on any path.
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serve) }

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if u, p, _ := r.BasicAuth(); s.opts.Username != "" && (u != s.opts.Username || p != s.opts.Password) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	method, params, err := client.DecodeCall(body, 0)
	w.Header().Set("Content-Type", "text/xml")
	if err != nil {
		_, _ = w.Write(client.EncodeFault(-32700, "parse error"))
		return
	}
	s.mu.Lock()
	v, fault := s.dispatch(method, params)
	s.mu.Unlock()
	if fault != nil {
		_, _ = w.Write(client.EncodeFault(fault.Code, fault.Message))
		return
	}
	out, err := client.EncodeResponse(v)
	if err != nil {
		_, _ = w.Write(client.EncodeFault(-32603, err.Error()))
		return
	}
	_, _ = w.Write(out)
}

func noMethod(m string) *client.Fault {
	return &client.Fault{Code: -506, Message: "Method '" + m + "' not defined"}
}

var noHash = &client.Fault{Code: -501, Message: "Could not find info-hash."}

type handler func(s *Server, params []any) (any, *client.Fault)

var handlers map[string]handler

func init() {
	handlers = map[string]handler{
		"system.client_version":         func(s *Server, _ []any) (any, *client.Fault) { return s.p.Version, nil },
		"system.library_version":        func(s *Server, _ []any) (any, *client.Fault) { return s.p.Library, nil },
		"system.multicall":              (*Server).multicallSystem,
		"directory.default":             func(*Server, []any) (any, *client.Fault) { return "/downloads", nil },
		"method.get":                    (*Server).methodGet,
		"d.multicall2":                  (*Server).multicall,
		"t.multicall":                   (*Server).trackers,
		"f.multicall":                   (*Server).files,
		"load.start":                    loader(true, false),
		"load.normal":                   loader(false, false),
		"load.raw_start":                loader(true, true),
		"load.raw":                      loader(false, true),
		"throttle.global_down.max_rate": func(s *Server, _ []any) (any, *client.Fault) { return s.downLimit, nil },
		"throttle.global_up.max_rate":   func(s *Server, _ []any) (any, *client.Fault) { return s.upLimit, nil },
		"throttle.global_down.max_rate.set": func(s *Server, p []any) (any, *client.Fault) {
			s.downLimit = num(p, 1)
			return int64(0), nil
		},
		"throttle.global_up.max_rate.set": func(s *Server, p []any) (any, *client.Fault) {
			s.upLimit = num(p, 1)
			return int64(0), nil
		},
		"throttle.global_down.rate": func(*Server, []any) (any, *client.Fault) { return int64(2048), nil },
		"throttle.global_up.rate":   func(*Server, []any) (any, *client.Fault) { return int64(512), nil },
	}
}

func num(p []any, i int) int64 {
	if i < len(p) {
		n, _ := p[i].(int64)
		return n
	}
	return 0
}

func str(p []any, i int) string {
	if i < len(p) {
		v, _ := p[i].(string)
		return v
	}
	return ""
}

func (s *Server) dispatch(method string, params []any) (any, *client.Fault) {
	if slices.Contains(s.p.Missing, method) {
		return nil, noMethod(method)
	}
	if h, ok := handlers[method]; ok {
		return h(s, params)
	}
	if cmd, ok := strings.CutPrefix(method, "d."); ok && len(params) > 0 {
		t, found := s.torrents[strings.ToUpper(str(params, 0))]
		if !found {
			return nil, noHash
		}
		return s.command(t, cmd, params[1:])
	}
	return nil, noMethod(method)
}

func (s *Server) multicallSystem(params []any) (any, *client.Fault) {
	calls, _ := params[0].([]any)
	out := []any{}
	for _, c := range calls {
		m, _ := c.(map[string]any)
		name, _ := m["methodName"].(string)
		args, _ := m["params"].([]any)
		v, fault := s.dispatch(name, args)
		if fault != nil {
			out = append(out, map[string]any{"faultCode": int64(fault.Code), "faultString": fault.Message})
			continue
		}
		out = append(out, []any{v})
	}
	return out, nil
}

func (s *Server) methodGet(params []any) (any, *client.Fault) {
	if str(params, 1) != "event.download.erased" {
		return nil, &client.Fault{Code: -503, Message: "Key not found."}
	}
	m := map[string]any{"!_download_list": "ui.focus.erase="}
	if s.p.EraseData {
		m["erasedata"] = "branch=d.custom5=,d.directory_base=..."
	}
	return m, nil
}

func (s *Server) hashes(view string) []string {
	var out []string
	for h, t := range s.torrents {
		if view == "main" || (view == "started" && t.Started) {
			out = append(out, h)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) multicall(params []any) (any, *client.Fault) {
	rows := []any{}
	for _, h := range s.hashes(str(params, 1)) {
		row := []any{}
		for _, c := range params[2:] {
			cmd, _ := c.(string)
			v, fault := s.command(s.torrents[h], strings.TrimSuffix(strings.TrimPrefix(cmd, "d."), "="), nil)
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

// command runs one d.* command on a download.
func (s *Server) command(t *Torrent, cmd string, args []any) (any, *client.Fault) {
	if v, ok := getter(t, cmd, s.free); ok {
		return v, nil
	}
	if field, ok := strings.CutSuffix(cmd, ".set"); ok && strings.HasPrefix(field, "custom") {
		t.Custom[field] = str(args, 0)
		return int64(0), nil
	}
	if strings.HasPrefix(cmd, "custom") {
		return t.Custom[cmd], nil
	}
	return s.action(t, cmd, args)
}

func getter(t *Torrent, cmd string, free int64) (any, bool) {
	v, ok := map[string]any{
		"hash": t.Hash, "name": t.Name, "state": b2i(t.Started), "is_active": b2i(t.Active), "is_open": b2i(t.Open),
		"complete": b2i(t.Complete), "is_hash_checking": b2i(t.Checking), "message": t.Message, "size_bytes": t.Size,
		"completed_bytes": t.Done, "left_bytes": t.Size - t.Done, "down.rate": t.DownRate, "up.rate": int64(0),
		"ratio": int64(0), "peers_complete": int64(2), "peers_accounted": int64(4), "priority": t.Priority,
		"is_multi_file": int64(1), "directory": t.Dir + "/" + t.Name, "free_diskspace": free,
	}[cmd]
	return v, ok
}

func (s *Server) action(t *Torrent, cmd string, args []any) (any, *client.Fault) {
	switch cmd {
	case "start":
		t.Started, t.Active, t.Open = true, true, true
	case "stop":
		t.Started, t.Active = false, false
	case "pause":
		t.Active = false
	case "resume":
		t.Active = t.Started
	case "erase":
		t.ErasedData = s.p.EraseData && t.Custom["custom5"] == "1"
		s.erased = append(s.erased, *t)
		delete(s.torrents, t.Hash)
	case "priority.set":
		t.Priority = num(args, 0)
	case "check_hash":
		t.Checking = true
	case "update_priorities", "tracker_announce", "tracker_announce.force":
	default:
		return nil, noMethod("d." + cmd)
	}
	return int64(0), nil
}

func loader(start, raw bool) handler {
	return func(s *Server, params []any) (any, *client.Fault) {
		if str(params, 0) != "" {
			return nil, &client.Fault{Code: -501, Message: "Unsupported target type found."}
		}
		hash, name, ok := loadedHash(params[1], raw)
		if !ok {
			return nil, &client.Fault{Code: -503, Message: "Could not create download."}
		}
		t := &Torrent{Hash: hash, Name: name, Dir: "/downloads", Size: 1000, Priority: 2, Custom: map[string]string{}}
		for _, c := range params[2:] {
			cmd, _ := c.(string)
			applyLoadCommand(t, cmd)
		}
		t.Started, t.Active, t.Open = start, start, start
		s.torrents[hash] = t
		return int64(0), nil
	}
}

func loadedHash(payload any, raw bool) (hash, name string, ok bool) {
	if raw {
		b, isBytes := payload.([]byte)
		h, err := client.TorrentHash(b)
		return h, "from-file", isBytes && err == nil
	}
	v, _ := payload.(string)
	h, err := client.MagnetHash(v)
	name = "from-magnet"
	if u, perr := url.Parse(v); perr == nil && u.Query().Get("dn") != "" {
		name = u.Query().Get("dn")
	}
	return h, name, err == nil
}

// applyLoadCommand applies a "d.x.set=value" command given when loading.
func applyLoadCommand(t *Torrent, c string) {
	key, val, _ := strings.Cut(c, "=")
	val = strings.TrimSuffix(strings.TrimPrefix(val, `"`), `"`)
	val = strings.ReplaceAll(strings.ReplaceAll(val, `\"`, `"`), `\\`, `\`)
	switch {
	case key == "d.directory.set":
		t.Dir = val
	case strings.HasPrefix(key, "d.custom"):
		t.Custom[strings.TrimSuffix(strings.TrimPrefix(key, "d."), ".set")] = val
	}
}

func (s *Server) trackers(params []any) (any, *client.Fault) {
	if _, ok := s.torrents[strings.ToUpper(str(params, 0))]; !ok {
		return nil, noHash
	}
	return []any{
		[]any{"https://tracker.example/announce/SECRETPASSKEY?x=1", int64(1), int64(10), int64(3), int64(1), int64(0)},
		[]any{"dht://", int64(1), int64(0), int64(0), int64(0), int64(1)},
	}, nil
}

func (s *Server) files(params []any) (any, *client.Fault) {
	if _, ok := s.torrents[strings.ToUpper(str(params, 0))]; !ok {
		return nil, noHash
	}
	return []any{[]any{"e01.mkv", int64(1000), int64(5), int64(10), int64(1)}}, nil
}
