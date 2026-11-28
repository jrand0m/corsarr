package music

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
)

// Mount registers the artist and album endpoints (search endpoints are
// mounted by the music automation).
func (s *Service) Mount(m api.Mounter) {
	m.Handle("GET /api/v1/artists", s.Enabled(s.list))
	m.Handle("POST /api/v1/artists", s.Enabled(s.add))
	m.Handle("GET /api/v1/artists/lookup", s.Enabled(s.lookup))
	m.Handle("GET /api/v1/artists/{id}", s.Enabled(s.get))
	m.Handle("PUT /api/v1/artists/{id}", s.Enabled(s.update))
	m.Handle("DELETE /api/v1/artists/{id}", s.Enabled(s.remove))
	m.Handle("POST /api/v1/artists/{id}/refresh", s.Enabled(s.refresh))
	m.Handle("GET /api/v1/albums/{id}", s.Enabled(s.album))
	m.Handle("PUT /api/v1/albums/monitor", s.Enabled(s.monitor))
	m.Handle("GET /api/v1/music/profiles", profiles)
}

// Enabled answers 409 while music is disabled; the mode is read per request.
func (s *Service) Enabled(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mode := s.d.Modes.Of(domain.KindMusic); mode == domain.ModeDisabled {
			WriteErr(w, domain.ModeError{Kind: domain.KindMusic, Mode: mode})
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

// PathID parses a positive id path value.
func PathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || v <= 0 {
		api.WriteError(w, http.StatusBadRequest, "invalid", "id must be a positive number", "id")
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
	ar, created, err := s.Add(r.Context(), req)
	if err != nil {
		WriteErr(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	api.WriteJSON(w, status, ar)
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	if id, ok := PathID(w, r); ok {
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
	id, ok := PathID(w, r)
	var p Patch
	if !ok || !api.DecodeJSON(w, r, &p) {
		return
	}
	ar, err := s.Update(r.Context(), id, p)
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, ar)
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

func (s *Service) refresh(w http.ResponseWriter, r *http.Request) {
	id, ok := PathID(w, r)
	if !ok {
		return
	}
	if _, err := s.Refresh(r.Context(), id); err != nil {
		WriteErr(w, err)
		return
	}
	s.writeDetail(w, r, id)
}

func (s *Service) album(w http.ResponseWriter, r *http.Request) {
	id, ok := PathID(w, r)
	if !ok {
		return
	}
	d, err := s.AlbumPage(r.Context(), id)
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, d)
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
	n, err := s.SetAlbumsMonitored(r.Context(), b.IDs, b.Monitored)
	if err != nil {
		WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]int{"updated": n})
}

type profileView struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Cutoff    string `json:"cutoff"`
	IsDefault bool   `json:"isDefault"`
}

// profiles lists the music quality profiles and the metadata profiles.
func profiles(w http.ResponseWriter, _ *http.Request) {
	quality := []profileView{}
	for _, p := range decide.ProfilesFor(domain.KindMusic) {
		key := p.Key()
		quality = append(quality, profileView{Key: key, Name: p.Spec().Name, Cutoff: string(p.Spec().Cutoff), IsDefault: key == DefaultProfileKey})
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"quality": quality, "metadata": domain.MetadataProfiles})
}
