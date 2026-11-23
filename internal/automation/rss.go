package automation

import (
	"context"
	"errors"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
)

// SyncRSS reads every indexer's feed from its cursor, matches each release
// against the wanted index, decides, grabs the best per item and only then
// advances the cursor (the cursor moves only once automation has processed
// the batch). A failed indexer keeps its cursor and does not stop the others.
func (s *Service) SyncRSS(ctx context.Context) error {
	if s.searchable() != nil {
		return nil // disabled kind: no feed reads
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	for _, ix := range s.d.Indexers() {
		errs = append(errs, s.syncOne(ctx, ix))
	}
	return errors.Join(errs...)
}

func (s *Service) syncOne(ctx context.Context, ix Indexer) error {
	cursor, err := s.d.Cursors.Get(ctx, ix.Key, domain.KindMovie)
	if err != nil {
		return err
	}
	rels, next, err := ix.Impl.Feed(ctx, domain.KindMovie, cursor)
	if err != nil {
		return err
	}
	for _, pick := range s.bestPerMovie(ctx, rels) {
		if _, err := s.grab(ctx, pick.movie, pick.d, domain.OriginRSS, false); !errors.Is(err, ErrShadowed) {
			s.warn("rss grab", err, "movie", pick.movie.ID)
		}
	}
	if next == cursor {
		return nil
	}
	return s.d.Cursors.Set(ctx, ix.Key, domain.KindMovie, next)
}

type rssPick struct {
	movie domain.Movie
	d     Decided
	all   []Decided
}

// bestPerMovie decides every matching (release, wanted movie) pair and keeps
// the best accepted release per movie.
func (s *Service) bestPerMovie(ctx context.Context, rels []domain.Release) []rssPick {
	byMovie := map[int64]*rssPick{}
	var order []int64
	var matched []domain.Release
	for _, rel := range rels {
		entries := s.index.Match(parse.ParseRelease(rel.Title))
		if len(entries) > 0 {
			matched = append(matched, rel)
		}
		for _, e := range entries {
			p, ok := byMovie[e.Movie.ID]
			if !ok {
				p = &rssPick{movie: e.Movie}
				byMovie[e.Movie.ID], order = p, append(order, e.Movie.ID)
			}
			p.all = append(p.all, Decided{Release: rel, Decision: s.decideOne(ctx, e.Movie, rel)})
		}
	}
	s.remember(ctx, matched)
	var out []rssPick
	for _, id := range order {
		p := byMovie[id]
		if top, _, ok := best(p.all); ok {
			p.d = top
			out = append(out, *p)
		}
	}
	return out
}
