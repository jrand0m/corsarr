package tvauto

import (
	"context"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/tv"
)

// InteractiveEpisode streams every decision for one episode (allowed before
// it airs: the availability reason shows on each row).
func (s *Service) InteractiveEpisode(ctx context.Context, id int64, em automation.Emitter) (automation.Summary, error) {
	if err := s.searchable(); err != nil {
		return automation.Summary{}, err
	}
	e, err := s.d.TV.Episode(ctx, id)
	if err != nil {
		return automation.Summary{}, err
	}
	sc, err := s.scopeOf(ctx, e.SeriesID)
	if err != nil {
		return automation.Summary{}, err
	}
	return s.interactive(ctx, sc, tv.EpisodeRequest(sc.series, e, domain.OriginInteractive), em), nil
}

// InteractiveSeason streams every decision for a season search.
func (s *Service) InteractiveSeason(ctx context.Context, seriesID int64, season int, em automation.Emitter) (automation.Summary, error) {
	if err := s.searchable(); err != nil {
		return automation.Summary{}, err
	}
	sc, err := s.scopeOf(ctx, seriesID)
	if err != nil {
		return automation.Summary{}, err
	}
	eps := inSeason(sc.episodes, season)
	if len(eps) == 0 {
		return automation.Summary{}, tv.ErrNotFound
	}
	return s.interactive(ctx, sc, tv.SeasonRequest(sc.series, season, eps, domain.OriginInteractive), em), nil
}

func (s *Service) interactive(ctx context.Context, sc scope, req domain.SearchRequest, em automation.Emitter) automation.Summary {
	ds, sum := s.collect(ctx, sc, req, em)
	_, sum.Accepted, _ = automation.Best(ds)
	return sum
}

// GrabRelease grabs a release from a recent search by hand for a series:
// the release is decided again against the episodes it covers; accepted
// and "allowed" overrides go through, "confirm" needs confirm, "never" is
// refused.
func (s *Service) GrabRelease(ctx context.Context, seriesID int64, releaseID string, confirm bool) (domain.Grab, automation.Decided, error) {
	return s.GrabReleaseTo(ctx, seriesID, releaseID, confirm, "")
}

// GrabReleaseTo is GrabRelease sent to one download client; an empty key
// picks the first usable client for the release's protocol.
func (s *Service) GrabReleaseTo(ctx context.Context, seriesID int64, releaseID string, confirm bool, to domain.ObjectKey) (domain.Grab, automation.Decided, error) {
	sc, err := s.scopeOf(ctx, seriesID)
	if err != nil {
		return domain.Grab{}, automation.Decided{}, err
	}
	rel, ok := s.recent.Get(ctx, releaseID)
	if !ok {
		return domain.Grab{}, automation.Decided{}, automation.ErrUnknownRelease
	}
	d := s.decideOne(ctx, sc, rel)
	if err := automation.OverrideAllowed(d.Decision, confirm); err != nil {
		return domain.Grab{}, d, err
	}
	g, err := s.grab(ctx, sc, d, domain.OriginInteractive, !d.Decision.Accepted, to)
	return g, d, err
}
