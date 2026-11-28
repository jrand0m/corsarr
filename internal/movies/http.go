package movies

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/domain"
)

// Mount registers the movie endpoints (search endpoints are mounted by automation).
func (s *Service) Mount(m api.Mounter) {
	m.Handle("GET /api/v1/movies", s.Enabled(s.list))
	m.Handle("POST /api/v1/movies", s.Enabled(s.add))
	m.Handle("GET /api/v1/movies/lookup", s.Enabled(s.lookup))
	m.Handle("GET /api/v1/movies/{id}", s.Enabled(s.get))
	m.Handle("PUT /api/v1/movies/{id}", s.Enabled(s.update))
	m.Handle("GET /api/v1/movies/{id}/move", s.Enabled(s.previewMove))
	m.Handle("DELETE /api/v1/movies/{id}", s.Enabled(s.remove))
}

// Enabled answers 409 while movies are disabled; the mode
// is read per request, so a config change applies without a restart.
func (s *Service) Enabled(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mode := s.d.Modes.Of(domain.KindMovie); mode == domain.ModeDisabled {
			WriteErr(w, domain.ModeError{Kind: domain.KindMovie, Mode: mode})
			return
		}
		h(w, r)
	}
}

// WriteErr maps module errors onto the API error shape.
func WriteErr(w http.ResponseWriter, err error) {
	var fe FieldError
	var me domain.ModeError
	switch {
	case errors.As(err, &me):
		api.WriteError(w, http.StatusConflict, "kind_mode", err.Error(), "")
	case errors.As(err, &fe):
		api.WriteError(w, http.StatusBadRequest, "invalid", fe.Msg, fe.Field)
	case errors.Is(err, ErrNotFound):
		api.WriteError(w, http.StatusNotFound, "not_found", err.Error(), "id")
	case errors.Is(err, ErrNoMetadata):
		api.WriteError(w, http.StatusServiceUnavailable, "no_metadata", err.Error(), "")
	default:
		api.WriteError(w, http.StatusBadGateway, "upstream", err.Error(), "")
	}
}

// PathID parses the {id} path value.
func PathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		api.WriteError(w, http.StatusBadRequest, "invalid", "id must be a positive number", "id")
		return 0, false
	}
	return id, true
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	cursor, limit, ok := api.Page(w, r)
	if !ok {
		return
	}
	items, next, err := s.List(r.Context(), cursor, limit)
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WritePage(w, items, next)
}

func (s *Service) lookup(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if term == "" {
		api.WriteError(w, http.StatusBadRequest, "invalid", "term is required", "term")
		return
	}
	recs, err := s.Lookup(r.Context(), term)
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": recs})
}

func (s *Service) add(w http.ResponseWriter, r *http.Request) {
	var req AddRequest
	if !api.DecodeJSON(w, r, &req) {
		return
	}
	m, created, err := s.Add(r.Context(), req)
	if err != nil {
		WriteErr(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	api.WriteJSON(w, status, m)
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	id, ok := PathID(w, r)
	if !ok {
		return
	}
	m, err := s.Get(r.Context(), id)
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, m)
}

func (s *Service) update(w http.ResponseWriter, r *http.Request) {
	id, ok := PathID(w, r)
	var p Patch
	if !ok || !api.DecodeJSON(w, r, &p) {
		return
	}
	m, err := s.Update(r.Context(), id, p)
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, m)
}

func (s *Service) previewMove(w http.ResponseWriter, r *http.Request) {
	id, ok := PathID(w, r)
	if !ok {
		return
	}
	plan, err := s.PreviewMove(r.Context(), id, r.URL.Query().Get("rootFolder"))
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, plan)
}

func (s *Service) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := PathID(w, r)
	if !ok {
		return
	}
	if err := s.Delete(r.Context(), id, r.URL.Query().Get("deleteFiles") == "true"); err != nil {
		WriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
