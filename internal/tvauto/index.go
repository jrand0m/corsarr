// Package tvauto is the TV side of automation: the wanted index over
// episodes, searches on add and on air, season-pack preference, the TV RSS
// sync, re-search after failures, interactive search and manual grabs. It
// reuses the kind-neutral pieces of the automation package.
package tvauto

import (
	"sort"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/parse"
	"github.com/jrand0m/corsarr/internal/tv"
)

// WantedReason is the pure rule: the series and the episode are monitored,
// the episode has aired, and it is missing or below the profile's cutoff.
func WantedReason(s domain.Series, e domain.Episode, cur *decide.CurrentFile, now time.Time) automation.Reason {
	if !s.Monitored || !e.Monitored || !tv.IsAvailable(e, now) {
		return automation.ReasonNotWanted
	}
	if cur == nil {
		return automation.ReasonMissing
	}
	if p, ok := tv.ProfileByKey(s.ProfileKey); ok && automation.BelowCutoff(p, *cur) {
		return automation.ReasonCutoffUnmet
	}
	return automation.ReasonNotWanted
}

// Entry is one wanted episode.
type Entry struct {
	Episode   tv.EpisodeView      `json:"episode"`
	Series    tv.SeriesRef        `json:"series"`
	Reason    automation.Reason   `json:"reason"`
	Available bool                `json:"available"`
	Current   *decide.CurrentFile `json:"current,omitempty"`
}

// Index is the in-memory wanted index: wanted episodes by id, and every
// series by normalised title for RSS matching.
type Index struct {
	mu       sync.RWMutex
	episodes map[int64]Entry
	bySeries map[int64]map[int64]bool // series → wanted episode ids
	series   map[int64]domain.Series
	byKey    map[string][]int64 // title key → series ids
}

// NewIndex returns an empty index.
func NewIndex() *Index {
	return &Index{episodes: map[int64]Entry{}, bySeries: map[int64]map[int64]bool{}, series: map[int64]domain.Series{},
		byKey: map[string][]int64{}}
}

// PutSeries files a series and replaces its wanted episodes.
func (x *Index) PutSeries(s domain.Series, wanted []Entry) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.dropSeries(s.ID)
	x.series[s.ID] = s
	for _, k := range tv.IndexKeys(s) {
		x.byKey[k] = append(x.byKey[k], s.ID)
	}
	for _, e := range wanted {
		x.putEpisode(e)
	}
}

// PutEpisode files one episode (dropping it when not wanted).
func (x *Index) PutEpisode(e Entry) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.dropEpisode(e.Series.ID, e.Episode.ID)
	x.putEpisode(e)
}

func (x *Index) putEpisode(e Entry) {
	if e.Reason == automation.ReasonNotWanted {
		return
	}
	x.episodes[e.Episode.ID] = e
	if x.bySeries[e.Series.ID] == nil {
		x.bySeries[e.Series.ID] = map[int64]bool{}
	}
	x.bySeries[e.Series.ID][e.Episode.ID] = true
}

// RemoveSeries drops a series and its episodes.
func (x *Index) RemoveSeries(id int64) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.dropSeries(id)
}

func (x *Index) dropSeries(id int64) {
	for eid := range x.bySeries[id] {
		delete(x.episodes, eid)
	}
	delete(x.bySeries, id)
	s, ok := x.series[id]
	if !ok {
		return
	}
	delete(x.series, id)
	for _, k := range tv.IndexKeys(s) {
		x.byKey[k] = without(x.byKey[k], id)
		if len(x.byKey[k]) == 0 {
			delete(x.byKey, k)
		}
	}
}

func (x *Index) dropEpisode(seriesID, id int64) {
	delete(x.episodes, id)
	delete(x.bySeries[seriesID], id)
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

// Wanted reports whether an episode is in the index.
func (x *Index) Wanted(id int64) (Entry, bool) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	e, ok := x.episodes[id]
	return e, ok
}

// Match returns the series a parsed release names.
func (x *Index) Match(p parse.ParsedRelease) []domain.Series {
	x.mu.RLock()
	defer x.mu.RUnlock()
	seen := map[int64]bool{}
	var out []domain.Series
	for _, t := range p.Titles {
		for _, id := range x.byKey[movies.Normalize(t)] {
			if s := x.series[id]; !seen[id] && tv.Matches(s, p) {
				seen[id] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// Page lists wanted episodes of one reason by ascending episode id after cursor.
func (x *Index) Page(reason automation.Reason, cursor int64, limit int) ([]Entry, int64) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	ids := make([]int64, 0, len(x.episodes))
	for id, e := range x.episodes {
		if id > cursor && e.Reason == reason {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var next int64
	if len(ids) > limit {
		ids, next = ids[:limit], ids[limit-1]
	}
	out := make([]Entry, len(ids))
	for i, id := range ids {
		out[i] = x.episodes[id]
	}
	return out, next
}

// Len is the number of wanted episodes.
func (x *Index) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.episodes)
}
