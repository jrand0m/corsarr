package tvauto

import (
	"context"
	"errors"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
	"github.com/jrand0m/corsarr/internal/queue"
	"github.com/jrand0m/corsarr/internal/tv"
)

// Summary is the result of one or more searches.
type Summary struct {
	Searches int `json:"searches"`
	automation.Summary
}

func (s *Summary) add(o automation.Summary) {
	s.Searches++
	s.Indexers += o.Indexers
	s.Failed += o.Failed
	s.Releases += o.Releases
	s.Accepted += o.Accepted
	s.Grabbed = first(s.Grabbed, o.Grabbed)
	s.Shadowed = first(s.Shadowed, o.Shadowed)
}

func (s *Summary) merge(o Summary) {
	n := s.Searches + o.Searches
	s.add(o.Summary)
	s.Searches = n
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// scope is one series with the episodes a search may cover.
type scope struct {
	series   domain.Series
	episodes []domain.Episode
	files    map[int64]*decide.CurrentFile
}

func (s *Service) scopeOf(ctx context.Context, seriesID int64) (scope, error) {
	ser, err := s.d.TV.Series(ctx, seriesID)
	if err != nil {
		return scope{}, err
	}
	eps, err := s.d.TV.Episodes(ctx, seriesID)
	return scope{series: ser, episodes: eps, files: map[int64]*decide.CurrentFile{}}, err
}

func (sc scope) byID(id int64) (domain.Episode, bool) {
	for _, e := range sc.episodes {
		if e.ID == id {
			return e, true
		}
	}
	return domain.Episode{}, false
}

// targets gathers what the decision needs about each covered episode.
func (s *Service) targets(ctx context.Context, sc scope, covered []domain.Episode, rel domain.Release) []tv.Target {
	profile, _ := tv.ProfileByKey(sc.series.ProfileKey)
	out := make([]tv.Target, len(covered))
	for i, e := range covered {
		cur, ok := sc.files[e.ID]
		if !ok {
			cur = s.current(ctx, e)
			sc.files[e.ID] = cur
		}
		out[i] = tv.Target{Episode: e, Current: cur, Context: s.context(ctx, e, profile, rel)}
	}
	return out
}

func (s *Service) context(ctx context.Context, e domain.Episode, p decide.QualityProfile, rel domain.Release) decide.Context {
	c := decide.Context{Now: s.d.Now(), Payload: s.d.Payload}
	var err1, err2, err3 error
	c.Blocklisted, err1 = s.d.Blocklist.Has(ctx, e.Ref(), domain.IdentityOf(rel))
	c.GrabbedBefore, err2 = s.d.Queue.GrabbedBefore(ctx, e.Ref(), rel)
	c.QueuedEqualOrBetter, err3 = s.queuedEqualOrBetter(ctx, e, p, rel)
	s.warn("decision lookups", errors.Join(err1, err2, err3), "episode", e.ID)
	return c
}

func (s *Service) queuedEqualOrBetter(ctx context.Context, e domain.Episode, p decide.QualityProfile, rel domain.Release) (bool, error) {
	queued, err := s.d.Queue.QueuedQualities(ctx, e.Ref())
	if err != nil || len(queued) == 0 {
		return false, err
	}
	cand, _ := decide.QualityOf(parse.ParseRelease(rel.Title))
	candRank, _ := p.Rank(cand.ID)
	for _, name := range queued {
		if rank, ok := automation.RankByName(p, name); !ok || rank >= candRank {
			return true, nil
		}
	}
	return false, nil
}

// decideOne decides a release for the episodes of the scope it covers.
func (s *Service) decideOne(ctx context.Context, sc scope, rel domain.Release) automation.Decided {
	covered := tv.Covered(parse.ParseRelease(rel.Title).Episode, sc.episodes)
	profile, _ := tv.ProfileByKey(sc.series.ProfileKey)
	return automation.Decided{Release: rel, Decision: tv.Decide(sc.series, profile, rel, s.targets(ctx, sc, covered, rel), nil)}
}

// collect queries every indexer, dedupes, decides each release and streams it.
func (s *Service) collect(ctx context.Context, sc scope, req domain.SearchRequest, em automation.Emitter) ([]automation.Decided, automation.Summary) {
	var out []automation.Decided
	var sum automation.Summary
	seen := map[string]bool{}
	for _, ix := range s.d.Indexers() {
		sum.Indexers++
		rels, err := ix.Impl.Search(ctx, req)
		if err != nil {
			sum.Failed++
			_ = em.Error(string(ix.Key), err)
			continue
		}
		for _, rel := range rels {
			if key := automation.IdentityKey(rel); !seen[key] {
				seen[key] = true
				_ = s.recent.Put(ctx, rel)
				d := s.decideOne(ctx, sc, rel)
				_ = em.Record(d)
				out = append(out, d)
			}
		}
	}
	sum.Releases = len(out)
	return out, sum
}

// run performs one automatic search and grabs the best accepted release.
func (s *Service) run(ctx context.Context, sc scope, req domain.SearchRequest, origin domain.SearchOrigin) automation.Summary {
	ds, sum := s.collect(ctx, sc, req, automation.NopEmitter)
	if top, n, ok := automation.Best(ds); ok {
		sum.Accepted = n
		switch _, err := s.grab(ctx, sc, top, origin, false, ""); {
		case err == nil:
			sum.Grabbed = top.Release.Title
		case errors.Is(err, automation.ErrShadowed):
			sum.Shadowed = top.Release.Title
		default:
			s.warn("grab", err, "release", top.Release.Title)
		}
	}
	s.publish(domain.EvSearchCompleted, "automation", s.items(sc, req.ItemRefs), domain.SearchEventData{Origin: origin,
		Indexers: sum.Indexers, Failed: sum.Failed, Releases: sum.Releases, Accepted: sum.Accepted, Grabbed: sum.Grabbed})
	return sum
}

// items names episode refs of the scope for events.
func (s *Service) items(sc scope, refs []domain.ItemRef) []domain.EventItem {
	out := make([]domain.EventItem, 0, len(refs))
	for _, r := range refs {
		e, ok := sc.byID(r.ID)
		if !ok {
			e = domain.Episode{ID: r.ID}
		}
		out = append(out, e.EventItem(sc.series.Title))
	}
	return out
}

// grab sends a decided release to the queue linked to every covered
// episode; in shadow mode it records decision.shadowed instead.
func (s *Service) grab(ctx context.Context, sc scope, d automation.Decided, origin domain.SearchOrigin, override bool, to domain.ObjectKey) (domain.Grab, error) {
	items := s.items(sc, d.Decision.ItemRefs)
	switch mode := s.d.Modes.Of(domain.KindTV); mode {
	case domain.ModeShadow:
		s.publish(domain.EvDecisionShadowed, automation.ActorOf(origin), items, domain.GrabEventData{Release: d.Release,
			Decision: d.Decision, Origin: origin, Override: override, Downloader: to})
		return domain.Grab{}, automation.ErrShadowed
	case domain.ModeDisabled:
		return domain.Grab{}, domain.ModeError{Kind: domain.KindTV, Mode: mode}
	}
	if len(items) == 0 {
		return domain.Grab{}, errors.New("release covers no episode of " + sc.series.Title)
	}
	g, err := s.d.Queue.Grab(ctx, queue.GrabRequest{Kind: domain.KindTV, Items: items, Release: d.Release,
		Decision: d.Decision, Origin: origin, Override: override, Downloader: to})
	if err == nil {
		for _, it := range items {
			s.refreshEpisode(ctx, it.Ref.ID)
		}
	}
	return g, err
}

// SearchEpisode searches one episode and grabs the best release.
func (s *Service) SearchEpisode(ctx context.Context, id int64, origin domain.SearchOrigin) (Summary, error) {
	if err := s.searchable(); err != nil {
		return Summary{}, err
	}
	e, err := s.d.TV.Episode(ctx, id)
	if err != nil {
		return Summary{}, err
	}
	sc, err := s.scopeOf(ctx, e.SeriesID)
	if err != nil {
		return Summary{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var sum Summary
	sum.add(s.run(ctx, sc, tv.EpisodeRequest(sc.series, e, origin), origin))
	return sum, nil
}

// SearchSeason searches a season pack first and falls back to the wanted
// episodes one by one when no pack is grabbed.
func (s *Service) SearchSeason(ctx context.Context, seriesID int64, season int, origin domain.SearchOrigin) (Summary, error) {
	if err := s.searchable(); err != nil {
		return Summary{}, err
	}
	sc, err := s.scopeOf(ctx, seriesID)
	if err != nil {
		return Summary{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.season(ctx, sc, season, origin), nil
}

func (s *Service) season(ctx context.Context, sc scope, season int, origin domain.SearchOrigin) Summary {
	var sum Summary
	sum.add(s.run(ctx, sc, tv.SeasonRequest(sc.series, season, inSeason(sc.episodes, season), origin), origin))
	if sum.Grabbed != "" || sum.Shadowed != "" {
		return sum
	}
	sum.merge(s.episodes(ctx, sc, s.wantedIn(sc, season), origin))
	return sum
}

func (s *Service) episodes(ctx context.Context, sc scope, eps []domain.Episode, origin domain.SearchOrigin) Summary {
	var sum Summary
	for _, e := range eps {
		if _, still := s.index.Wanted(e.ID); still {
			sum.add(s.run(ctx, sc, tv.EpisodeRequest(sc.series, e, origin), origin))
		}
	}
	return sum
}

func inSeason(eps []domain.Episode, season int) []domain.Episode {
	var out []domain.Episode
	for _, e := range eps {
		if e.Season == season {
			out = append(out, e)
		}
	}
	return out
}

func (s *Service) wantedIn(sc scope, season int) []domain.Episode {
	var out []domain.Episode
	for _, e := range inSeason(sc.episodes, season) {
		if _, ok := s.index.Wanted(e.ID); ok {
			out = append(out, e)
		}
	}
	return out
}

// SearchSeries searches every wanted episode of a series: a season pack
// first for fully aired seasons with enough episodes missing, single
// episodes otherwise.
func (s *Service) SearchSeries(ctx context.Context, seriesID int64, origin domain.SearchOrigin) (Summary, error) {
	if err := s.searchable(); err != nil {
		return Summary{}, err
	}
	sc, err := s.scopeOf(ctx, seriesID)
	if err != nil {
		return Summary{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var sum Summary
	for _, p := range Plan(sc.series, sc.episodes, s.wantedSet(sc), s.d.Now(), s.d.PackThreshold) {
		if p.Pack {
			sum.merge(s.season(ctx, sc, p.Season, origin))
			continue
		}
		sum.merge(s.episodes(ctx, sc, p.Episodes, origin))
	}
	return sum, nil
}

func (s *Service) wantedSet(sc scope) map[int64]bool {
	out := map[int64]bool{}
	for _, e := range sc.episodes {
		if _, ok := s.index.Wanted(e.ID); ok {
			out[e.ID] = true
		}
	}
	return out
}
