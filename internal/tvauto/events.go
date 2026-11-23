package tvauto

import (
	"context"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
)

// OnEvent keeps the wanted index current and runs the searches on add (when
// asked) and when an episode airs.
func (s *Service) OnEvent(ctx context.Context, ev domain.Event) {
	series, episodes := split(ev.Items)
	for _, id := range series {
		s.refreshSeries(ctx, id)
	}
	for _, id := range episodes {
		s.refreshEpisode(ctx, id)
	}
	if s.searchable() != nil {
		return
	}
	data, _ := ev.Data.(domain.ItemEventData)
	switch {
	case ev.Type == domain.EvItemAdded && data.SearchNow:
		for _, id := range series {
			_, err := s.SearchSeries(ctx, id, domain.OriginAutomatic)
			s.warn("search on add", err, "series", id)
		}
	case ev.Type == domain.EvItemAvailable:
		s.searchWanted(ctx, episodes)
	}
}

// OnFailure searches again for the episodes of a failed or removed
// download (the blocklist entry is added by the shared automation).
func (s *Service) OnFailure(ctx context.Context, ev domain.Event) {
	if !automation.Researches(ev) || s.searchable() != nil {
		return
	}
	_, episodes := split(ev.Items)
	for _, id := range episodes {
		s.refreshEpisode(ctx, id)
	}
	s.searchWanted(ctx, episodes)
}

// searchWanted searches each still-wanted episode; several episodes of one
// season are searched as that season.
func (s *Service) searchWanted(ctx context.Context, ids []int64) {
	done := map[[2]int64]bool{}
	for _, id := range ids {
		e, ok := s.index.Wanted(id)
		if !ok {
			continue
		}
		key := [2]int64{e.Series.ID, int64(e.Episode.Season)}
		if done[key] {
			continue
		}
		if s.siblings(ids, e) > 1 {
			done[key] = true
			_, err := s.SearchSeason(ctx, e.Series.ID, e.Episode.Season, domain.OriginAutomatic)
			s.warn("season search", err, "series", e.Series.ID)
			continue
		}
		_, err := s.SearchEpisode(ctx, id, domain.OriginAutomatic)
		s.warn("episode search", err, "episode", id)
	}
}

func (s *Service) siblings(ids []int64, e Entry) int {
	n := 0
	for _, id := range ids {
		if o, ok := s.index.Wanted(id); ok && o.Series.ID == e.Series.ID && o.Episode.Season == e.Episode.Season {
			n++
		}
	}
	return n
}

// split separates the TV series refs from the episode refs of an event.
func split(items []domain.EventItem) (series, episodes []int64) {
	for _, it := range items {
		switch {
		case it.Ref.Kind != domain.KindTV:
		case it.Ref.Variant == domain.VariantSeries:
			series = append(series, it.Ref.ID)
		default:
			episodes = append(episodes, it.Ref.ID)
		}
	}
	return series, episodes
}
