// Package tv is the TV media kind: series, seasons and episodes kept from
// the metadata plugin, monitoring modes, the episode availability timer and
// the kind's adapter for searching, matching, deciding and naming. The rules
// are pure functions; the service does the I/O and is the only writer of
// series and episodes.
package tv

import (
	"sort"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
)

// DefaultRuntime is the episode runtime assumed when the provider has none.
const DefaultRuntime = 45

// DefaultProfileKey is the profile a series gets when none is chosen.
const DefaultProfileKey = movies.DefaultProfileKey

// ProfileByKey finds a TV quality profile; TV ships the same profile names
// (and so the same keys) as movies, on the TV kind.
func ProfileByKey(key string) (decide.QualityProfile, bool) {
	return decide.Lookup(domain.KindTV, key)
}

// ProfileOrDefault is the series' profile, or the default one when its key
// no longer names a profile.
func ProfileOrDefault(key string) decide.QualityProfile {
	return movies.OrDefault(domain.KindTV, key, DefaultProfileKey)
}

// AvailableAt is when an episode can be searched: air time plus runtime;
// ok=false while the air date is unknown.
func AvailableAt(e domain.Episode) (time.Time, bool) {
	if e.AirAt == nil {
		return time.Time{}, false
	}
	rt := e.RuntimeMin
	if rt <= 0 {
		rt = DefaultRuntime
	}
	return e.AirAt.Add(time.Duration(rt) * time.Minute), true
}

// IsAvailable reports whether the episode has aired by now.
func IsAvailable(e domain.Episode, now time.Time) bool {
	at, ok := AvailableAt(e)
	return ok && !at.After(now)
}

// ApplyMode sets every episode's monitored flag for a monitoring mode and
// returns the season flags that decide what later episodes get. Specials
// stay unmonitored whatever the mode.
func ApplyMode(mode domain.MonitorMode, eps []domain.Episode, now time.Time) ([]domain.Episode, []domain.Season) {
	first, latest := seasonBounds(eps)
	out := make([]domain.Episode, len(eps))
	bySeason := map[int]bool{}
	for i, e := range eps {
		e.Monitored = e.Season > 0 && monitoredBy(mode, e, first, latest, now)
		out[i] = e
		bySeason[e.Season] = bySeason[e.Season] || e.Monitored
	}
	return out, seasonFlags(mode, bySeason, latest)
}

// ApplySeasons monitors exactly the listed seasons, every episode in them
// and later episodes of them; specials only when season 0 is listed.
func ApplySeasons(seasons []int, eps []domain.Episode) ([]domain.Episode, []domain.Season) {
	want := map[int]bool{}
	for _, n := range seasons {
		want[n] = true
	}
	out := make([]domain.Episode, len(eps))
	seen := map[int]bool{}
	var flags []domain.Season
	for i, e := range eps {
		e.Monitored = want[e.Season]
		out[i] = e
		if !seen[e.Season] {
			seen[e.Season] = true
			flags = append(flags, domain.Season{Number: e.Season, Monitored: want[e.Season]})
		}
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].Number < flags[j].Number })
	return out, flags
}

func monitoredBy(mode domain.MonitorMode, e domain.Episode, first, latest int, now time.Time) bool {
	switch mode {
	case domain.MonitorAll:
		return true
	case domain.MonitorFuture:
		return !IsAvailable(e, now)
	case domain.MonitorMissing:
		return e.FileID == 0
	case domain.MonitorExisting:
		return e.FileID != 0
	case domain.MonitorFirstSeason:
		return e.Season == first
	case domain.MonitorLatestSeason:
		return e.Season == latest
	case domain.MonitorPilot:
		return e.Season == first && e.Number == 1
	}
	return false
}

// seasonFlags: modes that follow the whole show keep every regular season
// open for new episodes; the others only the seasons they picked.
func seasonFlags(mode domain.MonitorMode, picked map[int]bool, latest int) []domain.Season {
	open := mode == domain.MonitorAll || mode == domain.MonitorFuture || mode == domain.MonitorMissing
	var out []domain.Season
	for n, any := range picked {
		mon := n > 0 && mode != domain.MonitorPilot && (open || any || (mode == domain.MonitorLatestSeason && n == latest))
		out = append(out, domain.Season{Number: n, Monitored: mon})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// seasonBounds is the first and latest regular season (0 when none).
func seasonBounds(eps []domain.Episode) (first, latest int) {
	for _, e := range eps {
		if e.Season <= 0 {
			continue
		}
		if first == 0 || e.Season < first {
			first = e.Season
		}
		latest = max(latest, e.Season)
	}
	return first, latest
}

// SortTitle drops a leading article and folds case for ordering.
func SortTitle(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	for _, a := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(t, a) {
			return strings.TrimSpace(t[len(a):])
		}
	}
	return t
}
