package tvauto

import (
	"errors"
	"net/http"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/tv"
)

// Mount registers the TV search, grab and wanted endpoints.
func (s *Service) Mount(m api.Mounter) {
	m.Handle("POST /api/v1/series/{id}/search", s.searchSeries)
	m.Handle("POST /api/v1/series/{id}/seasons/{n}/search", s.searchSeason)
	m.Handle("POST /api/v1/episodes/{id}/search", s.searchEpisode)
	m.Handle("GET /api/v1/episodes/{id}/releases", s.episodeReleases)
	m.Handle("GET /api/v1/series/{id}/seasons/{n}/releases", s.seasonReleases)
	m.Handle("POST /api/v1/series/{id}/grabs", s.grabRelease)
	m.Handle("GET /api/v1/wanted/missing/episodes", s.missing)
}

func (s *Service) searchSeries(w http.ResponseWriter, r *http.Request) {
	if id, ok := tv.PathInt(w, r, "id", false); ok {
		writeSummary(w)(s.SearchSeries(r.Context(), id, domain.OriginAutomatic))
	}
}

func (s *Service) searchSeason(w http.ResponseWriter, r *http.Request) {
	id, ok := tv.PathInt(w, r, "id", false)
	if !ok {
		return
	}
	if n, ok := tv.PathInt(w, r, "n", true); ok {
		writeSummary(w)(s.SearchSeason(r.Context(), id, int(n), domain.OriginAutomatic))
	}
}

func (s *Service) searchEpisode(w http.ResponseWriter, r *http.Request) {
	if id, ok := tv.PathInt(w, r, "id", false); ok {
		writeSummary(w)(s.SearchEpisode(r.Context(), id, domain.OriginAutomatic))
	}
}

func writeSummary(w http.ResponseWriter) func(Summary, error) {
	return func(sum Summary, err error) {
		if err != nil {
			tv.WriteErr(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, sum)
	}
}

// episodeReleases streams interactive search results as NDJSON.
func (s *Service) episodeReleases(w http.ResponseWriter, r *http.Request) {
	id, ok := tv.PathInt(w, r, "id", false)
	if !ok {
		return
	}
	if _, err := s.d.TV.Episode(r.Context(), id); err != nil {
		tv.WriteErr(w, err)
		return
	}
	s.stream(w, func(em automation.Emitter) (automation.Summary, error) {
		return s.InteractiveEpisode(r.Context(), id, em)
	})
}

func (s *Service) seasonReleases(w http.ResponseWriter, r *http.Request) {
	id, ok := tv.PathInt(w, r, "id", false)
	if !ok {
		return
	}
	n, ok := tv.PathInt(w, r, "n", true)
	if !ok {
		return
	}
	sc, err := s.scopeOf(r.Context(), id)
	if err == nil && len(inSeason(sc.episodes, int(n))) == 0 {
		err = tv.ErrNotFound
	}
	if err != nil {
		tv.WriteErr(w, err)
		return
	}
	s.stream(w, func(em automation.Emitter) (automation.Summary, error) {
		return s.InteractiveSeason(r.Context(), id, int(n), em)
	})
}

// stream checks the mode before the 200 header, then streams the search.
func (s *Service) stream(w http.ResponseWriter, run func(automation.Emitter) (automation.Summary, error)) {
	if err := s.searchable(); err != nil {
		tv.WriteErr(w, err)
		return
	}
	nd := api.NewNDJSON(w)
	sum, _ := run(nd) // the target exists and the mode was checked: no error left
	_ = nd.Done(map[string]any{"indexers": sum.Indexers, "failed": sum.Failed, "releases": sum.Releases, "accepted": sum.Accepted})
}

type grabBody struct {
	ReleaseID string `json:"releaseId"`
	Confirm   bool   `json:"confirm"`
	Season    *int   `json:"season"`
	EpisodeID *int64 `json:"episodeId"`
	// Downloader is the client to send it to; empty picks one.
	Downloader domain.ObjectKey `json:"downloader"`
}

// grabRelease grabs a release by hand; the episodes it covers come from the
// release itself (season and episodeId only name where the search ran).
func (s *Service) grabRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := tv.PathInt(w, r, "id", false)
	var b grabBody
	if !ok || !api.DecodeJSON(w, r, &b) {
		return
	}
	g, d, err := s.GrabReleaseTo(r.Context(), id, b.ReleaseID, b.Confirm, b.Downloader)
	switch {
	case errors.Is(err, automation.ErrUnknownRelease):
		api.WriteError(w, http.StatusNotFound, "not_found", err.Error(), "releaseId")
	case errors.Is(err, automation.ErrRefused):
		api.WriteJSON(w, http.StatusForbidden, map[string]any{"error": api.ErrorDetail{Code: "refused", Message: err.Error()}, "decision": d.Decision})
	case errors.Is(err, automation.ErrNeedsConfirm):
		api.WriteJSON(w, http.StatusConflict, map[string]any{"error": api.ErrorDetail{Code: "confirm_required", Message: err.Error()}, "decision": d.Decision})
	case errors.Is(err, automation.ErrShadowed):
		api.WriteJSON(w, http.StatusConflict, map[string]any{"error": api.ErrorDetail{Code: "shadowed", Message: err.Error()}, "decision": d.Decision})
	case err != nil:
		tv.WriteErr(w, err)
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
	api.WritePage(w, s.withArt(items), next)
}

// withArt points each entry's series poster at the artwork cache.
func (s *Service) withArt(items []Entry) []Entry {
	out := make([]Entry, len(items))
	for i, e := range items {
		e.Series = tv.WithArt(s.d.Art, e.Series)
		out[i] = e
	}
	return out
}
