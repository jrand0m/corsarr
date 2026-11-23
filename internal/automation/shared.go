package automation

import (
	"context"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
)

// Kind-neutral pieces other media kinds' automation reuses.

// Best is the top accepted decision after ranking, with the accepted count.
func Best(ds []Decided) (Decided, int, bool) { return best(ds) }

// IdentityKey dedupes the same release found on several indexers.
func IdentityKey(r domain.Release) string { return identityKey(r) }

// OverrideAllowed checks a manual grab of a possibly rejected decision.
func OverrideAllowed(d domain.Decision, confirm bool) error { return overrideAllowed(d, confirm) }

// ActorOf names the actor of a search origin.
func ActorOf(o domain.SearchOrigin) string { return actorOf(o) }

// BelowCutoff reports whether a file is below the profile's cutoff.
func BelowCutoff(p decide.QualityProfile, cur decide.CurrentFile) bool { return belowCutoff(p, cur) }

// RankByName ranks a quality name on a profile.
func RankByName(p decide.QualityProfile, name string) (int, bool) { return rankByName(p, name) }

// Researches reports whether a failure event asks for its items to be
// searched again (the blocklist entry is added by this package for every
// kind).
func Researches(ev domain.Event) bool {
	f, ok := failureOf(ev)
	return ok && f.research
}

// NopEmitter discards search progress.
var NopEmitter Emitter = nopEmitter{}

// Recent remembers releases shown by searches so a user can grab one by id
// later, also after a restart when it is backed by the database store.
type Recent struct {
	store RecentStore
	now   func() time.Time
}

// NewRecent wraps a RecentStore; a nil store keeps releases in memory only.
func NewRecent(store RecentStore, now func() time.Time) Recent {
	if store == nil {
		store = &memRecent{m: map[string]domain.Release{}}
	}
	if now == nil {
		now = time.Now
	}
	return Recent{store: store, now: now}
}

// Put remembers releases; a failure only costs a later "search again".
func (r Recent) Put(ctx context.Context, rels ...domain.Release) error {
	if len(rels) == 0 {
		return nil
	}
	now := r.now()
	return r.store.PutMany(ctx, rels, now, now.Add(-RecentTTL), RecentMax)
}

// Get finds a remembered release that has not expired.
func (r Recent) Get(ctx context.Context, id string) (domain.Release, bool) {
	rel, ok, err := r.store.Get(ctx, id, r.now().Add(-RecentTTL))
	return rel, ok && err == nil
}

// memRecent is the in-memory RecentStore (tests, no database).
type memRecent struct {
	mu    sync.Mutex
	order []string
	m     map[string]domain.Release
}

func (m *memRecent) PutMany(_ context.Context, rels []domain.Release, _, _ time.Time, max int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rel := range rels {
		if _, ok := m.m[rel.ID]; !ok {
			m.order = append(m.order, rel.ID)
		}
		m.m[rel.ID] = rel
	}
	for len(m.order) > max {
		delete(m.m, m.order[0])
		m.order = m.order[1:]
	}
	return nil
}

func (m *memRecent) Get(_ context.Context, id string, _ time.Time) (domain.Release, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rel, ok := m.m[id]
	return rel, ok, nil
}
