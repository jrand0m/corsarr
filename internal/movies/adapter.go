// Package movies is the movie media kind: the Movie repository
// front, lookup and add through the metadata plugin, availability with the
// `item.available` timer, and the MediaKind adapter (search requests,
// release matching, decision item state). Availability and the adapter are
// pure functions; the service does the I/O.
package movies

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
)

// ReleasedDelay is how long after the cinema date a movie counts as released
// when no digital or physical date is known.
const ReleasedDelay = 90 * 24 * time.Hour

// AvailableAt is when the movie meets its minimum availability; ok=false
// when no date allows it to be known.
func AvailableAt(m domain.Movie) (time.Time, bool) {
	switch m.MinAvailability {
	case domain.AvailAnnounced:
		return time.Time{}, true
	case domain.AvailInCinemas:
		if m.InCinemas != nil {
			return *m.InCinemas, true
		}
	}
	return released(m)
}

func released(m domain.Movie) (time.Time, bool) {
	if at, ok := earliest(m.Digital, m.Physical); ok {
		return at, true
	}
	if m.InCinemas != nil {
		return m.InCinemas.Add(ReleasedDelay), true
	}
	// No dates at all but a year: released once that year is over (1 January
	// of the next year, UTC). Old films often carry no release dates; a film
	// of the current or a future year without dates waits. No year and no
	// dates: never known, so never wanted under "released".
	if m.Year > 0 {
		return time.Date(m.Year+1, 1, 1, 0, 0, 0, 0, time.UTC), true
	}
	return time.Time{}, false
}

func earliest(a, b *time.Time) (time.Time, bool) {
	switch {
	case a == nil && b == nil:
		return time.Time{}, false
	case a == nil:
		return *b, true
	case b == nil || a.Before(*b):
		return *a, true
	}
	return *b, true
}

// IsAvailable reports whether the movie is available at now.
func IsAvailable(m domain.Movie, now time.Time) bool {
	at, ok := AvailableAt(m)
	return ok && !at.After(now)
}

// SearchRequest builds the movie search: TMDB and IMDb ids,
// title, original and alternate titles, year.
func SearchRequest(m domain.Movie, origin domain.SearchOrigin) domain.SearchRequest {
	ids := map[string]string{"tmdb": strconv.FormatInt(m.TMDBID, 10)}
	if m.IMDBID != "" {
		ids["imdb"] = m.IMDBID
	}
	return domain.SearchRequest{Kind: domain.KindMovie, Mode: domain.SearchMovie, Text: m.Title, Titles: titles(m),
		Year: m.Year, IDs: ids, Origin: origin, ItemRefs: []domain.ItemRef{m.Ref()}}
}

func titles(m domain.Movie) []string {
	out := []string{m.Title}
	if m.OriginalTitle != "" && m.OriginalTitle != m.Title {
		out = append(out, m.OriginalTitle)
	}
	return append(out, m.AltTitles...)
}

// Normalize folds a title for matching: lower case letters and digits only,
// "&" read as "and".
func Normalize(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "&", "and")
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

// IndexKeys are the normalised titles the wanted index files a movie under.
func IndexKeys(m domain.Movie) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range titles(m) {
		if k := Normalize(t); k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// Matches reports whether a parsed release names this movie: one of its
// titles and the year within ±1.
func Matches(m domain.Movie, p parse.ParsedRelease) bool {
	if !p.Year.Known || abs(p.Year.Value-m.Year) > 1 {
		return false
	}
	keys := IndexKeys(m)
	for _, t := range p.Titles {
		for _, k := range keys {
			if Normalize(t) == k {
				return true
			}
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Reject is the movie kind's own rejection rule: a release that does not
// name the movie (text-search fallbacks return other films).
func Reject(m domain.Movie, p parse.ParsedRelease) *domain.Reason {
	if Matches(m, p) {
		return nil
	}
	return &domain.Reason{RuleID: "movie.mismatch", Category: domain.ReasonPermanent,
		Message: "release does not match " + m.Title + " (" + strconv.Itoa(m.Year) + ")",
		Values:  map[string]any{"parsedTitle": p.Title, "parsedYear": p.Year.Value}}
}

// DecisionItem is the movie's state as the decision engine sees it.
func DecisionItem(m domain.Movie, profile decide.QualityProfile, current *decide.CurrentFile, now time.Time) decide.Item {
	return decide.Item{Ref: m.Ref(), Monitored: m.Monitored, Available: IsAvailable(m, now), RuntimeMinutes: m.RuntimeMin,
		Current: current, Profile: profile}
}

// Decide runs the shared engine plus the movie rejection rule.
func Decide(m domain.Movie, item decide.Item, rel domain.Release, ctx decide.Context, files []decide.PayloadFile) domain.Decision {
	p := parse.ParseRelease(rel.Title)
	d := decide.Decide(decide.Input{Release: rel, Parsed: p, Item: item, Files: files, Context: ctx})
	if why := Reject(m, p); why != nil {
		d.Reasons = append(d.Reasons, *why)
		d.Accepted, d.Override = false, domain.OverrideFor(d.Reasons)
	}
	return d
}
