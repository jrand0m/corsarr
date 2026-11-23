package tvauto

import (
	"sort"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/tv"
)

// Step is one search of a series search plan: a season pack (with episode
// fallback) or single episodes.
type Step struct {
	Season   int
	Pack     bool
	Episodes []domain.Episode // the wanted episodes
}

// Plan orders the searches for a series' wanted episodes (pure). A regular
// season whose episodes have all aired, with at least two wanted and a
// wanted share of its monitored episodes at or above threshold, is searched
// as a pack first; every other wanted episode is searched on its own.
func Plan(s domain.Series, eps []domain.Episode, wanted map[int64]bool, now time.Time, threshold float64) []Step {
	seasons := map[int][]domain.Episode{}
	var order []int
	for _, e := range eps {
		if _, ok := seasons[e.Season]; !ok {
			order = append(order, e.Season)
		}
		seasons[e.Season] = append(seasons[e.Season], e)
	}
	sort.Ints(order)
	var out []Step
	for _, n := range order {
		if step, ok := stepFor(n, seasons[n], wanted, now, threshold); ok {
			out = append(out, step)
		}
	}
	return out
}

func stepFor(season int, eps []domain.Episode, wanted map[int64]bool, now time.Time, threshold float64) (Step, bool) {
	step := Step{Season: season}
	monitored, aired := 0, true
	for _, e := range eps {
		if wanted[e.ID] {
			step.Episodes = append(step.Episodes, e)
		}
		if e.Monitored {
			monitored++
		}
		aired = aired && tv.IsAvailable(e, now)
	}
	if len(step.Episodes) == 0 {
		return step, false
	}
	share := float64(len(step.Episodes)) / float64(max(monitored, 1))
	step.Pack = season > 0 && aired && len(step.Episodes) >= 2 && share >= threshold
	return step, true
}
