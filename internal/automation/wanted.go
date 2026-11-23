package automation

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/parse"
)

// Reason is why an item is wanted.
type Reason string

const (
	ReasonMissing     Reason = "missing"
	ReasonCutoffUnmet Reason = "cutoff_unmet"
	ReasonNotWanted   Reason = ""
)

// Entry is one item in the wanted index.
type Entry struct {
	Movie     domain.Movie        `json:"movie"`
	Reason    Reason              `json:"reason"`
	Current   *decide.CurrentFile `json:"current,omitempty"`
	Available bool                `json:"available"`
	Search    *domain.SearchState `json:"search,omitempty"` // last searched, next eligible, last result
}

// WantedReason is the pure rule: monitored, available, and missing or
// below the profile's cutoff.
func WantedReason(m domain.Movie, cur *decide.CurrentFile, now time.Time) Reason {
	if !m.Monitored || !movies.IsAvailable(m, now) {
		return ReasonNotWanted
	}
	if cur == nil {
		return ReasonMissing
	}
	p, ok := movies.ProfileByKey(m.ProfileKey)
	if ok && belowCutoff(p, *cur) {
		return ReasonCutoffUnmet
	}
	return ReasonNotWanted
}

func belowCutoff(p decide.QualityProfile, cur decide.CurrentFile) bool {
	spec := p.Spec()
	cutRank, _ := p.Rank(spec.Cutoff)
	rank, ok := p.Rank(cur.Quality)
	return !ok || rank < cutRank || cur.FormatScore < spec.CutoffFormatScore
}

// Index is the one in-memory wanted index, keyed by movie
// id and by normalised title for RSS matching.
type Index struct {
	mu    sync.RWMutex
	byID  map[int64]Entry
	byKey map[string][]int64
}

// NewIndex returns an empty index.
func NewIndex() *Index { return &Index{byID: map[int64]Entry{}, byKey: map[string][]int64{}} }

// Put files a movie (wanted or not) under its id and title keys.
func (x *Index) Put(e Entry) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.drop(e.Movie.ID)
	if e.Reason == ReasonNotWanted {
		return
	}
	x.byID[e.Movie.ID] = e
	for _, k := range movies.IndexKeys(e.Movie) {
		x.byKey[k] = append(x.byKey[k], e.Movie.ID)
	}
}

// Remove drops a movie.
func (x *Index) Remove(id int64) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.drop(id)
}

func (x *Index) drop(id int64) {
	e, ok := x.byID[id]
	if !ok {
		return
	}
	delete(x.byID, id)
	for _, k := range movies.IndexKeys(e.Movie) {
		x.byKey[k] = without(x.byKey[k], id)
		if len(x.byKey[k]) == 0 {
			delete(x.byKey, k)
		}
	}
}

func without(ids []int64, id int64) []int64 {
	out := ids[:0]
	for _, v := range ids {
		if v != id {
			out = append(out, v)
		}
	}
	return out
}

// Get returns a wanted entry.
func (x *Index) Get(id int64) (Entry, bool) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	e, ok := x.byID[id]
	return e, ok
}

// Match returns the wanted movies a parsed release names (title key, year ±1).
func (x *Index) Match(p parse.ParsedRelease) []Entry {
	x.mu.RLock()
	defer x.mu.RUnlock()
	seen := map[int64]bool{}
	var out []Entry
	for _, t := range p.Titles {
		for _, id := range x.byKey[movies.Normalize(t)] {
			if e := x.byID[id]; !seen[id] && movies.Matches(e.Movie, p) {
				seen[id] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// Page lists wanted entries of one reason by ascending movie id after cursor.
func (x *Index) Page(reason Reason, cursor int64, limit int) ([]Entry, int64) {
	x.mu.RLock()
	ids := make([]int64, 0, len(x.byID))
	for id, e := range x.byID {
		if id > cursor && e.Reason == reason {
			ids = append(ids, id)
		}
	}
	x.mu.RUnlock()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var next int64
	if len(ids) > limit {
		ids, next = ids[:limit], ids[limit-1]
	}
	out := make([]Entry, 0, len(ids))
	for _, id := range ids {
		if e, ok := x.Get(id); ok {
			out = append(out, e)
		}
	}
	return out, next
}

// IDs lists the wanted movie ids.
func (x *Index) IDs() []int64 {
	x.mu.RLock()
	defer x.mu.RUnlock()
	out := make([]int64, 0, len(x.byID))
	for id := range x.byID {
		out = append(out, id)
	}
	return out
}

// Len is the number of wanted items.
func (x *Index) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.byID)
}

// entry derives a movie's index entry from its current file.
func (s *Service) entry(ctx context.Context, m domain.Movie) Entry {
	var cur *decide.CurrentFile
	if m.FileID != 0 {
		if f, ok, err := s.d.Files.File(ctx, m.FileID); err == nil && ok {
			cur = &decide.CurrentFile{Quality: decide.QualityID(f.Quality), FormatScore: f.FormatScore, Group: f.Group, Revision: f.Revision}
		}
	}
	now := s.d.Now()
	return Entry{Movie: m, Current: cur, Available: movies.IsAvailable(m, now), Reason: WantedReason(m, cur, now)}
}

// Rebuild fills the index from every movie (start-up).
func (s *Service) Rebuild(ctx context.Context) error {
	return s.d.Movies.Each(ctx, func(m domain.Movie) error {
		s.index.Put(s.entry(ctx, m))
		return nil
	})
}

// refresh re-derives one movie's entry (or drops it when gone).
func (s *Service) refresh(ctx context.Context, id int64) {
	m, err := s.d.Movies.Get(ctx, id)
	if err != nil {
		s.index.Remove(id)
		return
	}
	s.index.Put(s.entry(ctx, m))
}
