package musicauto

import (
	"errors"
	"net/http"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/music"
)

// Mount registers the music search, grab and wanted endpoints.
func (s *Service) Mount(m api.Mounter) {
	m.Handle("POST /api/v1/artists/{id}/search", s.searchArtist)
	m.Handle("POST /api/v1/albums/{id}/search", s.searchAlbum)
	m.Handle("GET /api/v1/albums/{id}/releases", s.releases)
	m.Handle("POST /api/v1/albums/{id}/grabs", s.grabRelease)
	m.Handle("GET /api/v1/wanted/missing/albums", s.missing)
}

func writeSummary(w http.ResponseWriter) func(Summary, error) {
	return func(sum Summary, err error) {
		if err != nil {
			music.WriteErr(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, sum)
	}
}

func (s *Service) searchArtist(w http.ResponseWriter, r *http.Request) {
	if id, ok := music.PathID(w, r); ok {
		writeSummary(w)(s.SearchArtist(r.Context(), id, domain.OriginAutomatic))
	}
}

func (s *Service) searchAlbum(w http.ResponseWriter, r *http.Request) {
	if id, ok := music.PathID(w, r); ok {
		writeSummary(w)(s.SearchAlbum(r.Context(), id, domain.OriginAutomatic))
	}
}

// releases streams interactive search results as NDJSON; the album and the
// mode are checked before the 200 header.
func (s *Service) releases(w http.ResponseWriter, r *http.Request) {
	id, ok := music.PathID(w, r)
	if !ok {
		return
	}
	err := s.searchable()
	if err == nil {
		_, err = s.d.Music.Album(r.Context(), id)
	}
	if err != nil {
		music.WriteErr(w, err)
		return
	}
	nd := api.NewNDJSON(w)
	sum, _ := s.Interactive(r.Context(), id, nd)
	_ = nd.Done(map[string]any{"indexers": sum.Indexers, "failed": sum.Failed, "releases": sum.Releases, "accepted": sum.Accepted})
}

type grabBody struct {
	ReleaseID string `json:"releaseId"`
	Confirm   bool   `json:"confirm"`
}

func (s *Service) grabRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := music.PathID(w, r)
	var b grabBody
	if !ok || !api.DecodeJSON(w, r, &b) {
		return
	}
	g, d, err := s.GrabRelease(r.Context(), id, b.ReleaseID, b.Confirm)
	refusal := map[error]struct {
		status int
		code   string
	}{automation.ErrRefused: {http.StatusForbidden, "refused"}, automation.ErrNeedsConfirm: {http.StatusConflict, "confirm_required"},
		automation.ErrShadowed: {http.StatusConflict, "shadowed"}}
	for target, out := range refusal {
		if errors.Is(err, target) {
			api.WriteJSON(w, out.status, map[string]any{"error": api.ErrorDetail{Code: out.code, Message: err.Error()}, "decision": d.Decision})
			return
		}
	}
	switch {
	case errors.Is(err, automation.ErrUnknownRelease):
		api.WriteError(w, http.StatusNotFound, "not_found", err.Error(), "releaseId")
	case err != nil:
		music.WriteErr(w, err)
	default:
		api.WriteJSON(w, http.StatusCreated, map[string]any{"grab": g, "decision": d.Decision})
	}
}

func (s *Service) missing(w http.ResponseWriter, r *http.Request) {
	cursor, limit, ok := api.Page(w, r)
	if !ok {
		return
	}
	items, next := s.index.Page(automation.ReasonMissing, cursor, limit)
	api.WritePage(w, items, next)
}
