package automation

import (
	"context"
	"errors"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/parse"
	"github.com/jrand0m/corsarr/internal/queue"
)

// Decided is one release with its full decision (an interactive search row).
type Decided struct {
	Release  domain.Release  `json:"release"`
	Decision domain.Decision `json:"decision"`
	// Swarm (torrents only) and Availability describe the payload for a
	// result card.
	Swarm        *decide.Swarm        `json:"swarm,omitempty"`
	Availability *decide.Availability `json:"availability,omitempty"`
}

// NewDecided pairs a release with its decision and payload facts.
func NewDecided(rel domain.Release, d domain.Decision) Decided {
	a := decide.AvailabilityOf(rel)
	return Decided{Release: rel, Decision: d, Swarm: decide.SwarmOf(rel), Availability: &a}
}

// Summary is the result of one search.
type Summary struct {
	Indexers int    `json:"indexers"`
	Failed   int    `json:"failed"`
	Releases int    `json:"releases"`
	Accepted int    `json:"accepted"`
	Skipped  int    `json:"skipped,omitempty"` // indexers skipped at their query limit
	Grabbed  string `json:"grabbed,omitempty"`
	Shadowed string `json:"shadowed,omitempty"` // would have been grabbed (shadow mode)
	// Automation is what automatic search would do with these results and
	// Rejections the most frequent rejection rules (interactive search).
	Automation *Verdict             `json:"automation,omitempty"`
	Rejections []domain.ReasonCount `json:"rejections,omitempty"`
}

// Explain adds to an interactive search's summary what automatic search
// would do with the same results.
func Explain(sum Summary, ds []Decided, mode domain.KindMode) Summary {
	_, sum.Accepted, _ = best(ds)
	v := Outcomes(ds, mode)
	sum.Automation, sum.Rejections = &v, Rejections(ds)
	return sum
}

// DoneFields is the interactive stream's closing summary.
func DoneFields(sum Summary) map[string]any {
	return map[string]any{"indexers": sum.Indexers, "failed": sum.Failed, "releases": sum.Releases, "accepted": sum.Accepted,
		"automation": sum.Automation, "rejections": sum.Rejections}
}

// Emitter receives search progress (interactive search streams it).
type Emitter interface {
	Record(v any) error
	Error(source string, err error) error
}

type nopEmitter struct{}

func (nopEmitter) Record(any) error          { return nil }
func (nopEmitter) Error(string, error) error { return nil }

// decideOne evaluates a release for a movie with the lookups the decision engine needs.
func (s *Service) decideOne(ctx context.Context, m domain.Movie, rel domain.Release) domain.Decision {
	profile := movies.ProfileOrDefault(m.ProfileKey)
	e := s.entry(ctx, m)
	c := decide.Context{Now: s.d.Now(), Payload: s.d.Payload}
	var err1, err2, err3 error
	c.Blocklisted, err1 = s.d.Blocklist.Has(ctx, m.Ref(), domain.IdentityOf(rel))
	c.GrabbedBefore, err2 = s.d.Queue.GrabbedBefore(ctx, m.Ref(), rel)
	c.QueuedEqualOrBetter, err3 = s.queuedEqualOrBetter(ctx, m, profile, rel)
	s.warn("decision lookups", errors.Join(err1, err2, err3), "movie", m.ID)
	return movies.Decide(m, movies.DecisionItem(m, profile, e.Current, s.d.Now()), rel, c, nil)
}

// queuedEqualOrBetter answers the queue lookup against the movie's profile.
func (s *Service) queuedEqualOrBetter(ctx context.Context, m domain.Movie, p decide.QualityProfile, rel domain.Release) (bool, error) {
	queued, err := s.d.Queue.QueuedQualities(ctx, m.Ref())
	if err != nil || len(queued) == 0 {
		return false, err
	}
	cand, _ := decide.QualityOf(parse.ParseRelease(rel.Title))
	candRank, _ := p.Rank(cand.ID)
	for _, name := range queued {
		if rank, ok := rankByName(p, name); !ok || rank >= candRank {
			return true, nil
		}
	}
	return false, nil
}

func rankByName(p decide.QualityProfile, name string) (int, bool) {
	for _, q := range decide.Ladder() {
		if q.Name == name {
			return p.Rank(q.ID)
		}
	}
	return 0, false
}

// collect queries every indexer with the movie's search request, dedupes,
// decides each release and streams it to em.
func (s *Service) collect(ctx context.Context, m domain.Movie, origin domain.SearchOrigin, em Emitter) ([]Decided, Summary) {
	req := movies.SearchRequest(m, origin)
	var out []Decided
	var sum Summary
	seen := map[string]bool{}
	for _, ix := range s.d.Indexers() {
		sum.Indexers++
		if err := s.allow(ctx, ix); err != nil {
			sum.Skipped++
			_ = em.Error(string(ix.Key), err)
			continue
		}
		rels, err := ix.Impl.Search(ctx, req)
		if err != nil {
			sum.Failed++
			_ = em.Error(string(ix.Key), err)
			continue
		}
		for _, rel := range rels {
			if key := identityKey(rel); !seen[key] {
				seen[key] = true
				out = append(out, s.decided(ctx, m, rel, em))
			}
		}
	}
	sum.Releases = len(out)
	rels := make([]domain.Release, len(out))
	for i, d := range out {
		rels[i] = d.Release
	}
	s.remember(ctx, rels)
	return out, sum
}

func (s *Service) decided(ctx context.Context, m domain.Movie, rel domain.Release, em Emitter) Decided {
	d := NewDecided(rel, s.decideOne(ctx, m, rel))
	_ = em.Record(d)
	return d
}

func identityKey(r domain.Release) string {
	if r.InfoHash != "" {
		return "h:" + r.InfoHash
	}
	return "id:" + r.ID
}

// best is the top accepted decision after ranking.
func best(ds []Decided) (Decided, int, bool) {
	byID := map[string]Decided{}
	var decisions []domain.Decision
	accepted := 0
	for _, d := range ds {
		byID[d.Decision.ReleaseID] = d
		decisions = append(decisions, d.Decision)
		if d.Decision.Accepted {
			accepted++
		}
	}
	ranked := decide.Rank(decisions)
	if accepted == 0 {
		return Decided{}, 0, false
	}
	return byID[ranked[0].ReleaseID], accepted, true
}

// Search runs an automatic search for one movie and grabs the best accepted
// release; it publishes search.completed.
func (s *Service) Search(ctx context.Context, movieID int64, origin domain.SearchOrigin) (Summary, error) {
	if err := s.searchable(); err != nil {
		return Summary{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.d.Movies.Get(ctx, movieID)
	if err != nil {
		return Summary{}, err
	}
	ds, sum := s.collect(ctx, m, origin, nopEmitter{})
	if top, n, ok := best(ds); ok {
		sum.Accepted = n
		switch _, err := s.grab(ctx, m, top, origin, false, ""); {
		case err == nil:
			sum.Grabbed = top.Release.Title
		case errors.Is(err, ErrShadowed):
			sum.Shadowed = top.Release.Title
		}
	}
	res := domain.SearchEventData{Origin: origin, Indexers: sum.Indexers, Failed: sum.Failed, Releases: sum.Releases,
		Accepted: sum.Accepted, Grabbed: sum.Grabbed, Rejections: Rejections(ds)}
	s.record(ctx, m, res)
	s.publish(domain.EvSearchCompleted, "automation", []domain.EventItem{m.EventItem()}, res)
	return sum, nil
}

// ErrShadowed reports a grab recorded as a decision.shadowed instead of sent.
var ErrShadowed = errors.New("shadow mode: the grab was recorded, not sent")

// searchable refuses searches for a disabled kind; shadow searches run.
func (s *Service) searchable() error {
	if mode := s.d.Modes.Of(domain.KindMovie); mode == domain.ModeDisabled {
		return domain.ModeError{Kind: domain.KindMovie, Mode: mode}
	}
	return nil
}

// grab sends a decided release to the queue. In shadow mode it records the
// would-be grab as decision.shadowed instead.
func (s *Service) grab(ctx context.Context, m domain.Movie, d Decided, origin domain.SearchOrigin, override bool, to domain.ObjectKey) (domain.Grab, error) {
	items := []domain.EventItem{m.EventItem()}
	switch mode := s.d.Modes.Of(domain.KindMovie); mode {
	case domain.ModeShadow:
		s.publish(domain.EvDecisionShadowed, actorOf(origin), items, domain.GrabEventData{Release: d.Release, Decision: d.Decision,
			Origin: origin, Override: override, Downloader: to})
		return domain.Grab{}, ErrShadowed
	case domain.ModeDisabled:
		return domain.Grab{}, domain.ModeError{Kind: domain.KindMovie, Mode: mode}
	}
	g, err := s.d.Queue.Grab(ctx, queue.GrabRequest{Kind: domain.KindMovie, Items: items,
		Release: d.Release, Decision: d.Decision, Origin: origin, Override: override, Downloader: to})
	if err == nil {
		s.refresh(ctx, m.ID)
	}
	return g, err
}

// Interactive streams every decision for a movie (allowed for unavailable
// movies: the availability reason shows on each row).
func (s *Service) Interactive(ctx context.Context, movieID int64, em Emitter) (Summary, error) {
	if err := s.searchable(); err != nil {
		return Summary{}, err
	}
	m, err := s.d.Movies.Get(ctx, movieID)
	if err != nil {
		return Summary{}, err
	}
	ds, sum := s.collect(ctx, m, domain.OriginInteractive, em)
	return Explain(sum, ds, s.d.Modes.Of(domain.KindMovie)), nil
}

// Errors of a manual grab.
var (
	ErrUnknownRelease = errors.New("this release is no longer in the recent search results; search again")
	ErrNeedsConfirm   = errors.New("release was rejected; confirm to grab anyway")
	ErrRefused        = errors.New("release can never be grabbed (safety)")
)

// GrabRelease grabs a release from a recent search by hand: accepted and
// "allowed" overrides go straight through, "confirm" needs confirm=true,
// "never" is refused.
func (s *Service) GrabRelease(ctx context.Context, movieID int64, releaseID string, confirm bool) (domain.Grab, Decided, error) {
	return s.GrabReleaseTo(ctx, movieID, releaseID, confirm, "")
}

// GrabReleaseTo is GrabRelease sent to one download client; an empty key
// picks the first usable client for the release's protocol.
func (s *Service) GrabReleaseTo(ctx context.Context, movieID int64, releaseID string, confirm bool, to domain.ObjectKey) (domain.Grab, Decided, error) {
	m, err := s.d.Movies.Get(ctx, movieID)
	if err != nil {
		return domain.Grab{}, Decided{}, err
	}
	rel, ok, err := s.d.Recent.Get(ctx, releaseID, s.d.Now().Add(-RecentTTL))
	if err != nil {
		return domain.Grab{}, Decided{}, err
	}
	if !ok {
		return domain.Grab{}, Decided{}, ErrUnknownRelease
	}
	d := NewDecided(rel, s.decideOne(ctx, m, rel))
	if err := overrideAllowed(d.Decision, confirm); err != nil {
		return domain.Grab{}, d, err
	}
	g, err := s.grab(ctx, m, d, domain.OriginInteractive, !d.Decision.Accepted, to)
	return g, d, err
}

func actorOf(o domain.SearchOrigin) string {
	if o == domain.OriginInteractive {
		return "user"
	}
	return "automation"
}

func overrideAllowed(d domain.Decision, confirm bool) error {
	switch {
	case d.Accepted:
		return nil
	case d.Override == domain.OverrideNever:
		return ErrRefused
	case d.Override == domain.OverrideConfirm && !confirm:
		return ErrNeedsConfirm
	}
	return nil
}
