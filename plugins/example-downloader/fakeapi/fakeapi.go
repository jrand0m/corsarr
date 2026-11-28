// Package fakeapi is a small in-memory download client with a JSON HTTP API.
// The example downloader module talks to it; tests serve it with httptest.
package fakeapi

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"sync"
)

// Download is one download as the API reports it.
type Download struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	SavePath string `json:"savePath"`
	State    string `json:"state"` // queued | downloading | paused | done
	Size     int64  `json:"size"`
	Done     int64  `json:"done"`
	Priority int    `json:"priority"` // queue position
	Level    string `json:"level"`
}

// Limits are the global limits in KiB/s.
type Limits struct {
	Down int64 `json:"down"`
	Up   int64 `json:"up"`
	Alt  bool  `json:"alt"`
}

// Server is the fake client. APIKey, when set, must be sent as X-Api-Key.
type Server struct {
	APIKey string

	mu         sync.Mutex
	downloads  map[string]*Download
	categories map[string]string
	limits     Limits
	paused     bool
	Deleted    []string // ids removed with their files
}

// New returns an empty client.
func New(apiKey string) *Server {
	return &Server{APIKey: apiKey, downloads: map[string]*Download{}, categories: map[string]string{}}
}

// Handler serves the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, _ *http.Request) { reply(w, map[string]string{"version": "1.0"}) })
	mux.HandleFunc("GET /api/v1/downloads", s.list)
	mux.HandleFunc("POST /api/v1/downloads", s.add)
	mux.HandleFunc("DELETE /api/v1/downloads/{id}", s.remove)
	mux.HandleFunc("POST /api/v1/downloads/{id}/{action}", s.act)
	mux.HandleFunc("GET /api/v1/downloads/{id}/files", s.files)
	mux.HandleFunc("GET /api/v1/downloads/{id}/trackers", s.trackers)
	mux.HandleFunc("PUT /api/v1/categories/{name}", s.category)
	mux.HandleFunc("POST /api/v1/pause-all", func(w http.ResponseWriter, _ *http.Request) { s.setPaused(true); reply(w, nil) })
	mux.HandleFunc("POST /api/v1/resume-all", func(w http.ResponseWriter, _ *http.Request) { s.setPaused(false); reply(w, nil) })
	mux.HandleFunc("GET /api/v1/limits", s.getLimits)
	mux.HandleFunc("PUT /api/v1/limits", s.putLimits)
	mux.HandleFunc("GET /api/v1/stats", s.stats)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.APIKey != "" && r.Header.Get("X-Api-Key") != s.APIKey {
			http.Error(w, "bad key", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if v == nil {
		v = map[string]any{}
	}
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	cats := r.URL.Query()["category"]
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Download{}
	for _, d := range s.downloads {
		if len(cats) == 0 || slices.Contains(cats, d.Category) {
			out = append(out, *d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	reply(w, out)
}

type addBody struct {
	Source   string `json:"source"` // URL or base64 content
	Name     string `json:"name"`
	Category string `json:"category"`
	SavePath string `json:"savePath"`
	Paused   bool   `json:"paused"`
}

func (s *Server) add(w http.ResponseWriter, r *http.Request) {
	var b addBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.Source == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sum := sha1.Sum([]byte(b.Source))
	id := hex.EncodeToString(sum[:8])
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.downloads[id]; !ok {
		state := "downloading"
		if b.Paused {
			state = "paused"
		}
		s.downloads[id] = &Download{ID: id, Name: b.Name, Category: b.Category, SavePath: b.SavePath, State: state,
			Size: 1 << 20, Priority: len(s.downloads), Level: "normal"}
	}
	reply(w, map[string]string{"id": id})
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) (*Download, bool) {
	d, ok := s.downloads[r.PathValue("id")]
	if !ok {
		http.Error(w, "no such download", http.StatusNotFound)
	}
	return d, ok
}

func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.get(w, r)
	if !ok {
		return
	}
	delete(s.downloads, d.ID)
	if r.URL.Query().Get("deleteFiles") == "true" {
		s.Deleted = append(s.Deleted, d.ID)
	}
	reply(w, nil)
}

var moves = map[string]func(int) int{
	"top": func(int) int { return -1 }, "bottom": func(int) int { return 1 << 30 },
	"up": func(p int) int { return p - 1 }, "down": func(p int) int { return p + 1 },
}

func (s *Server) act(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.get(w, r)
	if !ok {
		return
	}
	if !s.apply(d, r.PathValue("action"), body) {
		http.Error(w, "bad action", http.StatusBadRequest)
		return
	}
	reply(w, nil)
}

func (s *Server) apply(d *Download, action string, body map[string]string) bool {
	switch action {
	case "pause":
		d.State = "paused"
	case "resume":
		d.State = "downloading"
	case "recheck", "reannounce":
	case "priority":
		move, ok := moves[body["move"]]
		if !ok {
			return false
		}
		d.Priority = move(d.Priority)
	case "level":
		d.Level = body["level"]
	case "category":
		d.Category = body["category"]
	default:
		return false
	}
	return true
}

func (s *Server) files(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.get(w, r); ok {
		reply(w, []map[string]any{{"path": d.Name + "/" + d.Name + ".mkv", "size": d.Size}})
	}
}

func (s *Server) trackers(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.get(w, r); ok {
		reply(w, []map[string]any{{"url": "udp://tracker.example:1337", "status": "working", "seeds": 5, "peers": 2}})
	}
}

func (s *Server) category(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.categories[r.PathValue("name")] = body["savePath"]
	reply(w, nil)
}

func (s *Server) setPaused(p bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = p
}

func (s *Server) getLimits(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reply(w, s.limits)
}

func (s *Server) putLimits(w http.ResponseWriter, r *http.Request) {
	var l Limits
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		http.Error(w, "bad limits", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limits = l
	reply(w, nil)
}

func (s *Server) stats(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reply(w, map[string]any{"down": 1000, "up": 10, "free": 1 << 30, "paused": s.paused, "limits": s.limits})
}

// Categories returns the created categories and their save paths.
func (s *Server) Categories() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for k, v := range s.categories {
		out[k] = v
	}
	return out
}
