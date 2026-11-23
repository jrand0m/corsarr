package tv

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/domain"
)

// Mount registers the series endpoints (search endpoints are mounted by
// the TV automation).
func (s *Service) Mount(m api.Mounter) {
	m.Handle("GET /api/v1/series", s.Enabled(s.list))
	m.Handle("POST /api/v1/series", s.Enabled(s.add))
	m.Handle("GET /api/v1/series/lookup", s.Enabled(s.lookup))
	m.Handle("GET /api/v1/series/{id}", s.Enabled(s.get))
	m.Handle("PUT /api/v1/series/{id}", s.Enabled(s.update))
	m.Handle("DELETE /api/v1/series/{id}", s.Enabled(s.remove))
	m.Handle("PUT /api/v1/series/{id}/seasons/{n}", s.Enabled(s.season))
	m.Handle("POST /api/v1/series/{id}/refresh", s.Enabled(s.refresh))
	m.Handle("PUT /api/v1/episodes/monitor", s.Enabled(s.monitor))
	m.Handle("GET /api/v1/calendar/tv", s.Enabled(s.calendar))
}

// Enabled answers 409 while TV is disabled; the mode is read per request.
func (s *Service) Enabled(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mode := s.d.Modes.Of(domain.KindTV); mode == domain.ModeDisabled {
			WriteErr(w, domain.ModeError{Kind: domain.KindTV, Mode: mode})
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

// PathInt parses a positive (or, with zeroOK, non-negative) path value.
func PathInt(w http.ResponseWriter, r *http.Request, name string, zeroOK bool) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || v < 0 || (v == 0 && !zeroOK) {
		api.WriteError(w, http.StatusBadRequest, "invalid", name+" must be a positive number", name)
		return 0, false
	}
	return v, true
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
	items, err := s.Lookup(r.Context(), term)
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Service) add(w http.ResponseWriter, r *http.Request) {
	var req AddRequest
	if !api.DecodeJSON(w, r, &req) {
		return
	}
	ser, created, err := s.Add(r.Context(), req)
	if err != nil {
		WriteErr(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	api.WriteJSON(w, status, ser)
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	if id, ok := PathInt(w, r, "id", false); ok {
		s.writeDetail(w, r, id)
	}
}

func (s *Service) writeDetail(w http.ResponseWriter, r *http.Request, id int64) {
	d, err := s.Detail(r.Context(), id)
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, d)
}

func (s *Service) update(w http.ResponseWriter, r *http.Request) {
	id, ok := PathInt(w, r, "id", false)
	var p Patch
	if !ok || !api.DecodeJSON(w, r, &p) {
		return
	}
	ser, err := s.Update(r.Context(), id, p)
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, ser)
}

func (s *Service) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := PathInt(w, r, "id", false)
	if !ok {
		return
	}
	if err := s.Delete(r.Context(), id, r.URL.Query().Get("deleteFiles") == "true"); err != nil {
		WriteErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type monitoredBody struct {
	Monitored bool `json:"monitored"`
}

func (s *Service) season(w http.ResponseWriter, r *http.Request) {
	id, ok1 := PathInt(w, r, "id", false)
	if !ok1 {
		return
	}
	n, ok2 := PathInt(w, r, "n", true)
	var b monitoredBody
	if !ok2 || !api.DecodeJSON(w, r, &b) {
		return
	}
	if err := s.SetSeasonMonitored(r.Context(), id, int(n), b.Monitored); err != nil {
		WriteErr(w, err)
		return
	}
	s.writeDetail(w, r, id)
}

func (s *Service) refresh(w http.ResponseWriter, r *http.Request) {
	id, ok := PathInt(w, r, "id", false)
	if !ok {
		return
	}
	if _, err := s.Refresh(r.Context(), id); err != nil {
		WriteErr(w, err)
		return
	}
	s.writeDetail(w, r, id)
}

type monitorBody struct {
	IDs       []int64 `json:"ids"`
	Monitored bool    `json:"monitored"`
}

func (s *Service) monitor(w http.ResponseWriter, r *http.Request) {
	var b monitorBody
	if !api.DecodeJSON(w, r, &b) {
		return
	}
	if len(b.IDs) == 0 {
		api.WriteError(w, http.StatusBadRequest, "invalid", "ids is required", "ids")
		return
	}
	n, err := s.SetEpisodesMonitored(r.Context(), b.IDs, b.Monitored)
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]int{"updated": n})
}

// calendar lists episodes airing between start and end (default: the last
// 7 days to 28 days ahead).
func (s *Service) calendar(w http.ResponseWriter, r *http.Request) {
	now := s.d.Now()
	from, ok1 := timeParam(w, r, "start", now.AddDate(0, 0, -7))
	to, ok2 := timeParam(w, r, "end", now.AddDate(0, 0, 28))
	if !ok1 || !ok2 {
		return
	}
	items, err := s.Upcoming(r.Context(), from, to)
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func timeParam(w http.ResponseWriter, r *http.Request, name string, def time.Time) (time.Time, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, true
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid", name+" must be an RFC 3339 time", name)
	}
	return t, err == nil
}
