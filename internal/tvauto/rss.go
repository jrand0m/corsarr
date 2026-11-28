package tvauto

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
	"github.com/jrand0m/corsarr/internal/tv"
)

// SyncRSS reads every indexer's TV feed from its cursor, matches releases
// to series, decides those that cover a wanted episode, grabs the best per
// covered set (larger sets first, never two grabs for one episode) and only
// then advances the cursor. A failed indexer keeps its cursor.
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

func (s *Service) syncOne(ctx context.Context, ix automation.Indexer) error {
	cursor, err := s.d.Cursors.Get(ctx, ix.Key, domain.KindTV)
	if err != nil {
		return err
	}
	rels, next, err := ix.Impl.Feed(ctx, domain.KindTV, cursor)
	if err != nil {
		return err
	}
	grabbed := map[int64]bool{}
	for _, p := range s.picks(ctx, rels) {
		if overlaps(p.d.Decision.ItemRefs, grabbed) {
			continue
		}
		_, err := s.grab(ctx, p.scope, p.d, domain.OriginRSS, false, "")
		if err == nil || errors.Is(err, automation.ErrShadowed) {
			mark(p.d.Decision.ItemRefs, grabbed)
			continue
		}
		s.warn("rss grab", err, "series", p.scope.series.ID)
	}
	if next == cursor {
		return nil
	}
	return s.d.Cursors.Set(ctx, ix.Key, domain.KindTV, next)
}

type pick struct {
	scope scope
	d     automation.Decided
	all   []automation.Decided
}

// picks decides every release that covers a wanted episode and keeps the
// best accepted release per covered episode set.
func (s *Service) picks(ctx context.Context, rels []domain.Release) []pick {
	scopes := map[int64]scope{}
	byKey := map[string]*pick{}
	var order []string
	for _, rel := range rels {
		p := parse.ParseRelease(rel.Title)
		for _, ser := range s.index.Match(p) {
			sc, ok := s.scopeCached(ctx, scopes, ser.ID)
			covered := tv.Covered(p.Episode, sc.episodes)
			if !ok || !s.anyWanted(covered) {
				continue
			}
			key := setKey(ser.ID, covered)
			if byKey[key] == nil {
				byKey[key], order = &pick{scope: sc}, append(order, key)
			}
			_ = s.recent.Put(ctx, rel)
			byKey[key].all = append(byKey[key].all, s.decideOne(ctx, sc, rel))
		}
	}
	return ranked(byKey, order)
}

func ranked(byKey map[string]*pick, order []string) []pick {
	var out []pick
	for _, k := range order {
		p := byKey[k]
		if top, _, ok := automation.Best(p.all); ok {
			p.d = top
			out = append(out, *p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].d.Decision.ItemRefs) > len(out[j].d.Decision.ItemRefs) })
	return out
}

func (s *Service) scopeCached(ctx context.Context, cache map[int64]scope, id int64) (scope, bool) {
	if sc, ok := cache[id]; ok {
		return sc, true
	}
	sc, err := s.scopeOf(ctx, id)
	if err != nil {
		s.warn("rss series", err, "series", id)
		return sc, false
	}
	cache[id] = sc
	return sc, true
}

func (s *Service) anyWanted(eps []domain.Episode) bool {
	for _, e := range eps {
		if _, ok := s.index.Wanted(e.ID); ok {
			return true
		}
	}
	return false
}

func setKey(seriesID int64, eps []domain.Episode) string {
	parts := []string{strconv.FormatInt(seriesID, 10)}
	for _, e := range eps {
		parts = append(parts, strconv.FormatInt(e.ID, 10))
	}
	return strings.Join(parts, ",")
}

func overlaps(refs []domain.ItemRef, grabbed map[int64]bool) bool {
	for _, r := range refs {
		if grabbed[r.ID] {
			return true
		}
	}
	return false
}

func mark(refs []domain.ItemRef, grabbed map[int64]bool) {
	for _, r := range refs {
		grabbed[r.ID] = true
	}
}
