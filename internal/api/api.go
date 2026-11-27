// Package api is the `/api/v1` HTTP surface: stdlib ServeMux routing, one
// JSON error shape, admin/session/API
// key auth with a first-run flow, kernel endpoints, the SSE stream, an
// NDJSON streaming helper and the embedded UI with SPA fallback under an
// optional base path.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// AuthStore is what auth needs from persistence (store.Auth implements it).
type AuthStore interface {
	Admin(ctx context.Context) (store.AdminUser, bool, error)
	SetAdmin(ctx context.Context, username, hash string) error
	CreateSession(ctx context.Context, tokenHash string, expires time.Time) error
	SessionValid(ctx context.Context, tokenHash string, now time.Time) (bool, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	APIKey(ctx context.Context) (string, bool, error)
	SetAPIKey(ctx context.Context, key string) error
}

// HistoryStore pages History (store.History implements it).
type HistoryStore interface {
	List(ctx context.Context, cursor int64, limit int) ([]store.HistoryEntry, int64, error)
}

// Jobs is the scheduler surface the API uses.
type Jobs interface {
	List() []domain.JobState
	RunNow(name string) error
}

// HealthList lists integration health (health.Registry implements it).
type HealthList interface {
	List() []domain.IntegrationHealth
}

// Deps wires the API to the kernel. Config returns the current config (it
// changes on live reload).
type Deps struct {
	Version  string
	Started  time.Time
	BasePath string
	AuthMode string // password | local | proxy
	Auth     AuthStore
	History  HistoryStore
	Jobs     Jobs
	Health   HealthList
	Plugins  *plugin.Registry
	Config   func() *config.Config
	Events   http.Handler // SSE hub
	UI       fs.FS
	DBSize   func() int64
	Ready    func(ctx context.Context) error
	Log      *slog.Logger
	Now      func() time.Time
	// Mounts add module routes (movies, queue, …); every route they register
	// requires authentication.
	Mounts []func(Mounter)
}

// Mounter registers authenticated module routes on the API mux.
type Mounter interface {
	Handle(pattern string, h http.HandlerFunc)
}

// KeyMounter is implemented by the mounter modules receive: routes outside
// /api/v1 that authenticate by API key only (Torznab clients); deny answers
// unauthenticated requests in the route's own protocol.
type KeyMounter interface {
	HandleAPIKey(pattern string, h, deny http.HandlerFunc)
}

// KeyScope is what an API key presented by a client may do.
type KeyScope int

const (
	ScopeNone KeyScope = iota // no valid key
	ScopeRead                 // the read-only key
	ScopeFull                 // the API key
)

// ScopedMounter is implemented by the mounter modules receive: routes outside
// /api/v1 that authenticate by key (header X-Api-Key or a Bearer token, never
// the query) and do their own refusal; h sees the key's scope, ScopeNone
// included.
type ScopedMounter interface {
	HandleScoped(pattern string, h func(w http.ResponseWriter, r *http.Request, scope KeyScope))
}

func (m authedMounter) HandleScoped(pattern string, h func(http.ResponseWriter, *http.Request, KeyScope)) {
	m.s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) { h(w, r, m.s.KeyScope(r)) })
}

type authedMounter struct{ s *Server }

func (m authedMounter) HandleAPIKey(pattern string, h, deny http.HandlerFunc) {
	m.s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if m.s.validAPIKey(r) {
			h(w, r)
			return
		}
		deny(w, r)
	})
}

func (m authedMounter) Handle(pattern string, h http.HandlerFunc) {
	m.s.mux.Handle(pattern, m.s.requireAuth(h))
	if _, path, ok := strings.Cut(pattern, " /api/v1"); ok {
		m.s.paths = append(m.s.paths, path)
	}
}

// Server is the API handler.
type Server struct {
	d      Deps
	mux    *http.ServeMux
	apiKey atomic.Pointer[string]
	roKey  atomic.Pointer[string] // read-only key; "" when none is issued
	paths  []string               // module paths for the OpenAPI index
}

// New builds the handler tree. The API key must already exist (EnsureAPIKey).
func New(ctx context.Context, d Deps) (*Server, error) {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	s := &Server{d: d, mux: http.NewServeMux()}
	key, ok, err := d.Auth.APIKey(ctx)
	if err != nil || !ok {
		return nil, errors.Join(errors.New("api key missing; call EnsureAPIKey first"), err)
	}
	s.apiKey.Store(&key)
	if err := s.loadReadOnlyKey(ctx); err != nil {
		return nil, err
	}
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	open := s.mux.HandleFunc
	open("GET /api/v1/health", s.health)
	open("GET /api/v1/firstrun", s.firstRun)
	open("POST /api/v1/auth/setup", s.setup)
	open("POST /api/v1/auth/login", s.login)
	open("POST /api/v1/auth/logout", s.logout)
	open("GET /api/v1/auth/me", s.me)
	authed := func(pattern string, h http.HandlerFunc) { s.mux.Handle(pattern, s.requireAuth(h)) }
	authed("GET /api/v1/auth/apikey", s.getAPIKey)
	authed("POST /api/v1/auth/apikey/rotate", s.rotateAPIKey)
	authed("GET /api/v1/auth/apikey/readonly", s.getReadOnlyKey)
	authed("POST /api/v1/auth/apikey/readonly/rotate", s.rotateReadOnlyKey)
	authed("DELETE /api/v1/auth/apikey/readonly", s.revokeReadOnlyKey)
	authed("GET /api/v1/system/status", s.status)
	authed("GET /api/v1/config/schema", s.configSchema)
	authed("GET /api/v1/config/export", s.configExport)
	authed("GET /api/v1/plugins", s.plugins)
	authed("GET /api/v1/health/integrations", s.integrations)
	authed("GET /api/v1/jobs", s.jobs)
	authed("POST /api/v1/jobs/{name}/run", s.runJob)
	authed("GET /api/v1/history", s.history)
	authed("GET /api/v1/openapi.json", s.openapi)
	for _, mount := range s.d.Mounts {
		mount(authedMounter{s})
	}
	if s.d.Events != nil {
		s.mux.Handle("GET /api/v1/events", s.requireAuth(s.d.Events.ServeHTTP))
	}
	open("/api/", func(w http.ResponseWriter, _ *http.Request) {
		WriteError(w, http.StatusNotFound, "not_found", "no such endpoint", "")
	})
	open("/legacy/", func(w http.ResponseWriter, _ *http.Request) { // reserved for the compatibility API
		WriteError(w, http.StatusNotFound, "not_found", "legacy compatibility routes are reserved and not enabled", "")
	})
	if s.d.UI != nil {
		s.mux.Handle("/", spa(s.d.UI, s.d.BasePath))
	}
}

// Handler returns the root handler with the base path applied.
func (s *Server) Handler() http.Handler {
	h := http.Handler(s.mux)
	base := strings.TrimSuffix(s.d.BasePath, "/")
	if base == "" {
		return h
	}
	strip := http.StripPrefix(base, h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == base {
			http.Redirect(w, r, base+"/", http.StatusMovedPermanently)
			return
		}
		if !strings.HasPrefix(r.URL.Path, base+"/") {
			http.NotFound(w, r)
			return
		}
		strip.ServeHTTP(w, r)
	})
}

// ── JSON helpers ─────────────────────────────────────────────────────────────

// ErrorBody is the one error shape every endpoint returns.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail carries a stable code, a human message and an optional field.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// WriteError writes `{error:{code,message,field?}}`.
func WriteError(w http.ResponseWriter, status int, code, msg, field string) {
	WriteJSON(w, status, ErrorBody{Error: ErrorDetail{Code: code, Message: msg, Field: field}})
}

// WriteJSON writes v with status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// DecodeJSON reads a bounded JSON body, rejecting unknown fields.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		WriteError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error(), "")
		return false
	}
	return true
}

func (s *Server) internal(w http.ResponseWriter, err error) {
	s.d.Log.Error("api error", "err", err)
	WriteError(w, http.StatusInternalServerError, "internal", "internal error", "")
}
