package automation

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
)

// Mount registers search, grab, wanted and blocklist endpoints.
func (s *Service) Mount(m api.Mounter) {
	m.Handle("POST /api/v1/movies/{id}/search", s.search)
	m.Handle("GET /api/v1/movies/{id}/releases", s.releases)
	m.Handle("POST /api/v1/grabs", s.grabRelease)
	m.Handle("GET /api/v1/wanted/missing", s.missing)
	m.Handle("GET /api/v1/blocklist", s.blocklist)
	m.Handle("POST /api/v1/blocklist", s.addBlock)
	m.Handle("DELETE /api/v1/blocklist/{id}", s.removeBlock)
}

func (s *Service) search(w http.ResponseWriter, r *http.Request) {
	id, ok := movies.PathID(w, r)
	if !ok {
		return
	}
	sum, err := s.Search(r.Context(), id, domain.OriginAutomatic)
	if err != nil {
		movies.WriteErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, sum)
}

// releases streams interactive search results as NDJSON.
func (s *Service) releases(w http.ResponseWriter, r *http.Request) {
	id, ok := movies.PathID(w, r)
	if !ok {
		return
	}
	if _, err := s.d.Movies.Get(r.Context(), id); err != nil {
		movies.WriteErr(w, err)
		return
	}
	if err := s.searchable(); err != nil {
		movies.WriteErr(w, err)
		return
	}
	nd := api.NewNDJSON(w)
	sum, _ := s.Interactive(r.Context(), id, nd) // the movie exists: no error left
	_ = nd.Done(map[string]any{"indexers": sum.Indexers, "failed": sum.Failed, "releases": sum.Releases, "accepted": sum.Accepted})
}

type grabBody struct {
	MovieID   int64  `json:"movieId"`
	ReleaseID string `json:"releaseId"`
	Confirm   bool   `json:"confirm"`
}

func (s *Service) grabRelease(w http.ResponseWriter, r *http.Request) {
	var b grabBody
	if !api.DecodeJSON(w, r, &b) {
		return
	}
	g, d, err := s.GrabRelease(r.Context(), b.MovieID, b.ReleaseID, b.Confirm)
	switch {
	case errors.Is(err, ErrUnknownRelease):
		api.WriteError(w, http.StatusNotFound, "release_expired", err.Error(), "releaseId")
	case errors.Is(err, ErrRefused):
		api.WriteJSON(w, http.StatusForbidden, map[string]any{"error": api.ErrorDetail{Code: "refused", Message: err.Error()}, "decision": d.Decision})
	case errors.Is(err, ErrNeedsConfirm):
		api.WriteJSON(w, http.StatusConflict, map[string]any{"error": api.ErrorDetail{Code: "confirm_required", Message: err.Error()}, "decision": d.Decision})
	case errors.Is(err, ErrShadowed):
		api.WriteJSON(w, http.StatusConflict, map[string]any{"error": api.ErrorDetail{Code: "shadowed", Message: err.Error()}, "decision": d.Decision})
	case err != nil:
		movies.WriteErr(w, err)
	default:
		api.WriteJSON(w, http.StatusCreated, map[string]any{"grab": g, "decision": d.Decision})
	}
}

func (s *Service) missing(w http.ResponseWriter, r *http.Request) {
	cursor, limit, ok := api.Page(w, r)
	if !ok {
		return
	}
	items, next := s.index.Page(ReasonMissing, cursor, limit)
	for i := range items {
		if st, err := s.d.Searches.State(r.Context(), items[i].Movie.Ref()); err == nil && st.LastSearched != nil {
			items[i].Search = &st
		}
	}
	api.WritePage(w, items, next)
}

func (s *Service) blocklist(w http.ResponseWriter, r *http.Request) {
	cursor, limit, ok := api.Page(w, r)
	if !ok {
		return
	}
	items, next, err := s.d.Blocklist.List(r.Context(), cursor, limit)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "internal", err.Error(), "")
		return
	}
	api.WritePage(w, items, next)
}

type blockBody struct {
	MovieID int64                  `json:"movieId"`
	Release domain.ReleaseIdentity `json:"release"`
	Reason  string                 `json:"reason"`
}

func (s *Service) addBlock(w http.ResponseWriter, r *http.Request) {
	var b blockBody
	if !api.DecodeJSON(w, r, &b) {
		return
	}
	if b.Release.Title == "" {
		api.WriteError(w, http.StatusBadRequest, "invalid", "release.title is required", "release.title")
		return
	}
	m, err := s.d.Movies.Get(r.Context(), b.MovieID)
	if err != nil {
		movies.WriteErr(w, err)
		return
	}
	e, err := s.AddBlock(r.Context(), domain.BlocklistEntry{Item: m.Ref(), Release: b.Release, Source: "manual", Reason: b.Reason}, m.EventItem())
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "internal", err.Error(), "")
		return
	}
	api.WriteJSON(w, http.StatusCreated, e)
}

func (s *Service) removeBlock(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid", "id must be a number", "id")
		return
	}
	if _, ok, err := s.RemoveBlock(r.Context(), id); err != nil || !ok {
		api.WriteError(w, http.StatusNotFound, "not_found", "no blocklist entry "+r.PathValue("id"), "id")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
