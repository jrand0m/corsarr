package queue

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// Mount registers GET /api/v1/queue and POST /api/v1/queue/{id}/{action}.
func (s *Service) Mount(m api.Mounter) {
	m.Handle("GET /api/v1/queue", s.list)
	m.Handle("POST /api/v1/queue/{id}/{action}", s.action)
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	cursor, limit, ok := api.Page(w, r)
	if !ok {
		return
	}
	items, next, err := s.List(r.Context(), cursor, limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.Wake() // the Queue is open: poll now
	api.WritePage(w, items, next)
}

type actionBody struct {
	Priority plugin.Priority `json:"priority"`
	RemoveOptions
}

func (s *Service) action(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid", "id must be a number", "id")
		return
	}
	var body actionBody
	if r.ContentLength != 0 && !api.DecodeJSON(w, r, &body) {
		return
	}
	run, ok := map[string]func() error{
		"pause":    func() error { return s.Pause(r.Context(), id) },
		"resume":   func() error { return s.Resume(r.Context(), id) },
		"priority": func() error { return s.SetPriority(r.Context(), id, body.Priority) },
		"remove":   func() error { return s.Remove(r.Context(), id, body.RemoveOptions) },
		"hold":     func() error { return s.Hold(r.Context(), id, true) },
		"release":  func() error { return s.Hold(r.Context(), id, false) },
	}[r.PathValue("action")]
	if !ok {
		api.WriteError(w, http.StatusNotFound, "not_found", "action must be pause, resume, priority, remove, hold or release", "action")
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

func writeErr(w http.ResponseWriter, err error) {
	var me domain.ModeError
	switch {
	case errors.As(err, &me):
		api.WriteError(w, http.StatusConflict, "kind_mode", err.Error(), "")
	case errors.Is(err, ErrNotFound):
		api.WriteError(w, http.StatusNotFound, "not_found", err.Error(), "id")
	case errors.Is(err, ErrHeld):
		api.WriteError(w, http.StatusConflict, "held", err.Error(), "")
	case errors.Is(err, ErrNoDownloader):
		api.WriteError(w, http.StatusServiceUnavailable, "no_downloader", err.Error(), "")
	case plugin.ClassOf(err) == plugin.ClassConfig:
		api.WriteError(w, http.StatusBadRequest, "invalid", err.Error(), "")
	default:
		api.WriteError(w, http.StatusBadGateway, "upstream", err.Error(), "")
	}
}
