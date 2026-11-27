package queue

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// Mount registers the queue and downloader-control routes:
//
//	GET  /api/v1/queue                       ?downloader=&kind=&state=a,b
//	GET  /api/v1/queue/{id}                  files, trackers, state history
//	POST /api/v1/queue/{id}/{action}         pause|resume|priority|recheck|reannounce|category|remove|retry-import|hold|release
//	GET  /api/v1/downloaders                 health, protocols, capabilities, stats
//	GET  /api/v1/downloaders/{key}
//	POST /api/v1/downloaders/{key}/{action}  pause-all|resume-all|speed-limits|alt-speed|test
func (s *Service) Mount(m api.Mounter) {
	m.Handle("GET /api/v1/queue", s.list)
	m.Handle("GET /api/v1/queue/{id}", s.detail)
	m.Handle("POST /api/v1/queue/{id}/{action}", s.action)
	m.Handle("GET /api/v1/downloaders", s.listClients)
	m.Handle("GET /api/v1/downloaders/{key}", s.getClient)
	m.Handle("POST /api/v1/downloaders/{key}/{action}", s.clientAction)
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	cursor, limit, ok := api.Page(w, r)
	if !ok {
		return
	}
	f, err := parseFilter(r)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid", err.Error(), "")
		return
	}
	items, next, err := s.ListFiltered(r.Context(), f, cursor, limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.Wake() // the Queue is open: poll now
	api.WritePage(w, items, next)
}

var knownStates = map[domain.DownloadState]bool{
	domain.StateQueued: true, domain.StateDownloading: true, domain.StateStalled: true, domain.StateCompleted: true,
	domain.StateImportPending: true, domain.StateImporting: true, domain.StateImportBlocked: true,
	domain.StateImported: true, domain.StateFailed: true, domain.StateRemoved: true,
}

func parseFilter(r *http.Request) (Filter, error) {
	q := r.URL.Query()
	f := Filter{Downloader: domain.ObjectKey(q.Get("downloader"))}
	if k := q.Get("kind"); k != "" {
		kind, err := domain.ParseMediaKind(k)
		if err != nil {
			return f, err
		}
		f.Kind = kind
	}
	for _, st := range strings.Split(q.Get("state"), ",") {
		if st == "" {
			continue
		}
		if !knownStates[domain.DownloadState(st)] {
			return f, errors.New("unknown state " + strconv.Quote(st))
		}
		f.States = append(f.States, domain.DownloadState(st))
	}
	return f, nil
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid", "id must be a number", "id")
		return 0, false
	}
	return id, true
}

func (s *Service) detail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	d, err := s.Detail(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, d)
}

type actionBody struct {
	Priority plugin.Priority `json:"priority"`
	Category *string         `json:"category"`
	RemoveOptions
}

const actionNames = "pause, resume, priority, recheck, reannounce, category, remove, retry-import, hold or release"

func (s *Service) action(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body actionBody
	if r.ContentLength != 0 && !api.DecodeJSON(w, r, &body) {
		return
	}
	run, ok := s.actions(r, id, body)[r.PathValue("action")]
	if !ok {
		api.WriteError(w, http.StatusNotFound, "not_found", "action must be "+actionNames, "action")
		return
	}
	if err := run(); err != nil {
		writeErr(w, err)
		return
	}
	d, err := s.Get(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, d)
}

func (s *Service) actions(r *http.Request, id int64, body actionBody) map[string]func() error {
	ctx := r.Context()
	return map[string]func() error{
		"pause":        func() error { return s.Pause(ctx, id) },
		"resume":       func() error { return s.Resume(ctx, id) },
		"priority":     func() error { return s.SetPriority(ctx, id, body.Priority) },
		"recheck":      func() error { return s.Recheck(ctx, id) },
		"reannounce":   func() error { return s.Reannounce(ctx, id) },
		"category":     func() error { return s.setCategory(ctx, id, body.Category) },
		"remove":       func() error { return s.Remove(ctx, id, body.RemoveOptions) },
		"retry-import": func() error { return s.RetryImport(ctx, id) },
		"hold":         func() error { return s.Hold(ctx, id, true) },
		"release":      func() error { return s.Hold(ctx, id, false) },
	}
}

func (s *Service) listClients(w http.ResponseWriter, r *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": s.Clients(r.Context())})
}

func (s *Service) getClient(w http.ResponseWriter, r *http.Request) {
	v, err := s.Client(r.Context(), domain.ObjectKey(r.PathValue("key")))
	if err != nil {
		writeErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, v)
}

type clientBody struct {
	DownKiB int64 `json:"downKiB"`
	UpKiB   int64 `json:"upKiB"`
	On      *bool `json:"on"`
}

func (s *Service) clientAction(w http.ResponseWriter, r *http.Request) {
	key := domain.ObjectKey(r.PathValue("key"))
	var body clientBody
	if r.ContentLength != 0 && !api.DecodeJSON(w, r, &body) {
		return
	}
	ctx := r.Context()
	run, ok := map[string]func() error{
		"pause-all":  func() error { return s.PauseAll(ctx, key) },
		"resume-all": func() error { return s.ResumeAll(ctx, key) },
		"speed-limits": func() error {
			return s.SetSpeedLimits(ctx, key, plugin.SpeedLimits{DownKiB: body.DownKiB, UpKiB: body.UpKiB})
		},
		"alt-speed": func() error { _, err := s.SetAltSpeed(ctx, key, body.On); return err },
		"test":      func() error { return s.TestClient(ctx, key) },
	}[r.PathValue("action")]
	if !ok {
		api.WriteError(w, http.StatusNotFound, "not_found", "action must be pause-all, resume-all, speed-limits, alt-speed or test", "action")
		return
	}
	if err := run(); err != nil {
		writeErr(w, err)
		return
	}
	s.getClient(w, r)
}

// errorStatuses map service errors to statuses and codes, first match wins.
var errorStatuses = []struct {
	err    error
	status int
	code   string
}{
	{ErrNotFound, http.StatusNotFound, "not_found"},
	{ErrUnknownClient, http.StatusNotFound, "not_found"},
	{ErrHeld, http.StatusConflict, "held"},
	{ErrShadowed, http.StatusConflict, "shadowed"},
	{ErrNotImportable, http.StatusConflict, "not_importable"},
	{plugin.ErrUnsupported, http.StatusNotImplemented, "unsupported"},
	{ErrNoDownloader, http.StatusServiceUnavailable, "no_downloader"},
}

func writeErr(w http.ResponseWriter, err error) {
	var me domain.ModeError
	if errors.As(err, &me) {
		api.WriteError(w, http.StatusConflict, "kind_mode", err.Error(), "")
		return
	}
	for _, e := range errorStatuses {
		if errors.Is(err, e.err) {
			api.WriteError(w, e.status, e.code, err.Error(), "")
			return
		}
	}
	if plugin.ClassOf(err) == plugin.ClassConfig {
		api.WriteError(w, http.StatusBadRequest, "invalid", err.Error(), "")
		return
	}
	api.WriteError(w, http.StatusBadGateway, "upstream", err.Error(), "")
}
