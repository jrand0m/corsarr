package api

import (
	"errors"
	"net/http"
	"runtime"
	"sort"
	"strconv"

	"github.com/jrand0m/corsarr/internal/buildinfo"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/scheduler"
)

// health is liveness + readiness, unauthenticated.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{"status": "ok", "version": s.d.Version}
	if s.d.Ready != nil {
		if err := s.d.Ready(r.Context()); err != nil {
			body["status"], body["error"] = "unavailable", err.Error()
			WriteJSON(w, http.StatusServiceUnavailable, body)
			return
		}
	}
	WriteJSON(w, http.StatusOK, body)
}

// SystemStatus is GET /api/v1/system/status.
type SystemStatus struct {
	Version       string             `json:"version"`
	UptimeSeconds int64              `json:"uptimeSeconds"`
	RSSBytes      int64              `json:"rssBytes"`
	Goroutines    int                `json:"goroutines"`
	DBSizeBytes   int64              `json:"dbSizeBytes"`
	ConfigSource  config.Source      `json:"configSource"`
	EnabledKinds  []domain.MediaKind `json:"enabledKinds"`
	PluginCount   int                `json:"pluginCount"`
	Build         buildinfo.Info     `json:"build"`
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	st := SystemStatus{
		Build:   buildinfo.Get(s.d.Version),
		Version: s.d.Version, UptimeSeconds: int64(s.d.Now().Sub(s.d.Started).Seconds()),
		RSSBytes: rssBytes(), Goroutines: runtime.NumGoroutine(), EnabledKinds: []domain.MediaKind{},
	}
	if s.d.DBSize != nil {
		st.DBSizeBytes = s.d.DBSize()
	}
	if s.d.Plugins != nil {
		st.PluginCount = len(s.d.Plugins.List(""))
	}
	c := s.d.Config()
	st.ConfigSource = c.Source
	for k, m := range c.MediaKinds {
		if m != domain.ModeDisabled {
			st.EnabledKinds = append(st.EnabledKinds, k)
		}
	}
	sort.Slice(st.EnabledKinds, func(i, j int) bool { return st.EnabledKinds[i] < st.EnabledKinds[j] })
	WriteJSON(w, http.StatusOK, st)
}

func (s *Server) configSchema(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/schema+json")
	_, _ = w.Write(config.NewSchema(s.d.Plugins).JSONSchema())
}

func (s *Server) configExport(w http.ResponseWriter, r *http.Request) {
	opts := config.ExportOptions{Full: r.URL.Query().Get("full") == "true"}
	c := s.d.Config()
	switch r.URL.Query().Get("format") {
	case "", "yaml":
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(c.ExportYAML(opts))
	case "env":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(c.ExportEnv(opts))
	default:
		WriteError(w, http.StatusBadRequest, "invalid", "format must be yaml or env", "format")
	}
}

func (s *Server) plugins(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"items": s.d.Plugins.List("")})
}

func (s *Server) integrations(w http.ResponseWriter, _ *http.Request) {
	items := s.d.Health.List()
	if items == nil {
		items = []domain.IntegrationHealth{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) jobs(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"items": s.d.Jobs.List()})
}

func (s *Server) runJob(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.d.Jobs.RunNow(name); errors.Is(err, scheduler.ErrUnknownJob) {
		WriteError(w, http.StatusNotFound, "not_found", "no job named "+name, "name")
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	WriteJSON(w, http.StatusAccepted, map[string]any{"queued": name})
}

// history pages History newest first: ?cursor=<id>&limit=<1..200>.
func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	cursor, limit, ok := Page(w, r)
	if !ok {
		return
	}
	items, next, err := s.d.History.List(r.Context(), cursor, limit)
	if err != nil {
		s.internal(w, err)
		return
	}
	WritePage(w, items, next)
}

// Page parses the cursor paging parameters ?cursor=<id>&limit=<1..200>
// (default 50); on bad input it writes the error and returns ok=false.
func Page(w http.ResponseWriter, r *http.Request) (cursor int64, limit int, ok bool) {
	q := r.URL.Query()
	c, err1 := parseInt(q.Get("cursor"), 0)
	l, err2 := parseInt(q.Get("limit"), 50)
	if err1 != nil || err2 != nil || l < 1 || l > 200 || c < 0 {
		WriteError(w, http.StatusBadRequest, "invalid", "cursor must be ≥ 0 and limit 1..200", "")
		return 0, 0, false
	}
	return c, int(l), true
}

// WritePage writes one page `{items, nextCursor?}` (nextCursor absent on the last page).
func WritePage(w http.ResponseWriter, items any, next int64) {
	body := map[string]any{"items": items}
	if next > 0 {
		body["nextCursor"] = strconv.FormatInt(next, 10)
	}
	WriteJSON(w, http.StatusOK, body)
}

func parseInt(s string, def int64) (int64, error) {
	if s == "" {
		return def, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

// openapi is the API description: a minimal OpenAPI 3.1 index of the kernel
// paths; later lanes add their operations.
func (s *Server) openapi(w http.ResponseWriter, _ *http.Request) {
	paths := map[string]any{}
	for _, p := range append([]string{"/health", "/firstrun", "/auth/setup", "/auth/login", "/auth/logout", "/auth/me",
		"/auth/apikey", "/auth/apikey/rotate", "/system/status", "/config/schema", "/config/export", "/plugins",
		"/health/integrations", "/jobs", "/jobs/{name}/run", "/history", "/events"}, s.paths...) {
		paths[p] = map[string]any{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"openapi": "3.1.0", "info": map[string]any{"title": "corsarr", "version": s.d.Version},
		"servers": []any{map[string]any{"url": s.d.BasePath + "/api/v1"}}, "paths": paths,
	})
}
