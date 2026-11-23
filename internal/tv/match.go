package tv

import (
	"fmt"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/parse"
)

// Titles are the series' title and alternates, title first.
func Titles(s domain.Series) []string { return append([]string{s.Title}, s.AltTitles...) }

// IndexKeys are the normalised titles the wanted index files a series under.
func IndexKeys(s domain.Series) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range Titles(s) {
		if k := movies.Normalize(t); k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// Matches reports whether a parsed release names this series: one of its
// titles, and when the release states a year it is within one of the
// series' first year (remakes share titles).
func Matches(s domain.Series, p parse.ParsedRelease) bool {
	if p.Year.Known && s.Year > 0 && abs(p.Year.Value-s.Year) > 1 {
		return false
	}
	keys := IndexKeys(s)
	for _, t := range p.Titles {
		n := movies.Normalize(t)
		for _, k := range keys {
			if n == k {
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

// Covered lists the library episodes a release addresses: the numbered
// episodes, every episode of the packed seasons, or the episode that aired
// on a daily date.
func Covered(ep parse.Episode, eps []domain.Episode) []domain.Episode {
	var out []domain.Episode
	for _, e := range eps {
		if covers(ep, e) {
			out = append(out, e)
		}
	}
	return out
}

func covers(ep parse.Episode, e domain.Episode) bool {
	switch ep.Kind {
	case parse.EpisodeStandard:
		return len(ep.Seasons) == 1 && ep.Seasons[0] == e.Season && contains(ep.Episodes, e.Number)
	case parse.EpisodeSeason:
		return contains(ep.Seasons, e.Season)
	case parse.EpisodeDaily:
		return e.AirAt != nil && e.AirAt.Format(time.DateOnly) == ep.AirDate
	}
	return false
}

func contains(xs []int, n int) bool {
	for _, x := range xs {
		if x == n {
			return true
		}
	}
	return false
}

// IsPack reports whether a release is a whole-season pack.
func IsPack(p parse.ParsedRelease) bool { return p.Episode.Kind == parse.EpisodeSeason }

func ids(s domain.Series) map[string]string {
	out := map[string]string{"tmdb": strconv.FormatInt(s.TMDBID, 10)}
	if s.TVDBID > 0 {
		out["tvdb"] = strconv.FormatInt(s.TVDBID, 10)
	}
	if s.IMDBID != "" {
		out["imdb"] = s.IMDBID
	}
	return out
}

func refs(eps []domain.Episode) []domain.ItemRef {
	out := make([]domain.ItemRef, len(eps))
	for i, e := range eps {
		out[i] = e.Ref()
	}
	return out
}

// EpisodeRequest searches one episode by series ids, season and number; the
// text form ("Show S01E02") serves indexers without id search.
func EpisodeRequest(s domain.Series, e domain.Episode, origin domain.SearchOrigin) domain.SearchRequest {
	return domain.SearchRequest{Kind: domain.KindTV, Mode: domain.SearchTV, Text: s.Title + " " + e.Code(), Titles: Titles(s),
		Year: s.Year, IDs: ids(s), Season: e.Season, Episodes: []int{e.Number}, Origin: origin, ItemRefs: []domain.ItemRef{e.Ref()}}
}

// SeasonRequest searches a whole season (packs first).
func SeasonRequest(s domain.Series, season int, eps []domain.Episode, origin domain.SearchOrigin) domain.SearchRequest {
	return domain.SearchRequest{Kind: domain.KindTV, Mode: domain.SearchTV, Text: fmt.Sprintf("%s S%02d", s.Title, season),
		Titles: Titles(s), Year: s.Year, IDs: ids(s), Season: season, Origin: origin, ItemRefs: refs(eps)}
}

// Target is one covered episode with what the decision needs about it.
type Target struct {
	Episode domain.Episode
	Current *decide.CurrentFile
	Context decide.Context
}

// Decide runs the shared engine for every covered episode and returns one
// decision linked to all of them. Size limits apply per covered episode; a
// release is accepted when at least one covered episode accepts it, so a
// pack never downgrades an episode (the import skips those it does not
// improve). A release that names another show or no episode is rejected.
func Decide(s domain.Series, profile decide.QualityProfile, rel domain.Release, targets []Target, files []decide.PayloadFile) domain.Decision {
	p := parse.ParseRelease(rel.Title)
	if len(targets) == 0 {
		return rejected(rel, p, reasonNoEpisode(s, p))
	}
	per := rel
	per.Size = rel.Size / int64(len(targets))
	var pick domain.Decision
	for i, t := range targets {
		d := decide.Decide(decide.Input{Release: per, Parsed: p, Item: item(s, profile, t), Files: files, Context: t.Context})
		if i == 0 || d.Accepted && !pick.Accepted || !pick.Accepted && len(d.Reasons) < len(pick.Reasons) {
			pick = d
		}
	}
	pick.ItemRefs = targetRefs(targets)
	if !Matches(s, p) {
		pick.Reasons = append(pick.Reasons, *reasonMismatch(s, p))
		pick.Accepted, pick.Override = false, domain.OverrideFor(pick.Reasons)
	}
	return pick
}

func item(s domain.Series, profile decide.QualityProfile, t Target) decide.Item {
	runtime := t.Episode.RuntimeMin
	if runtime <= 0 {
		runtime = s.RuntimeMin // the episode's own runtime is often unknown
	}
	return decide.Item{Ref: t.Episode.Ref(), Monitored: s.Monitored && t.Episode.Monitored, Available: IsAvailable(t.Episode, t.Context.Now),
		RuntimeMinutes: runtime, Current: t.Current, Profile: profile}
}

func targetRefs(ts []Target) []domain.ItemRef {
	out := make([]domain.ItemRef, len(ts))
	for i, t := range ts {
		out[i] = t.Episode.Ref()
	}
	return out
}

func rejected(rel domain.Release, p parse.ParsedRelease, why *domain.Reason) domain.Decision {
	q, _ := decide.QualityOf(p)
	reasons := []domain.Reason{*why}
	return domain.Decision{ReleaseID: rel.ID, ItemRefs: []domain.ItemRef{}, Reasons: reasons, Quality: q.Name,
		Resolution: int(p.Resolution.Value), Override: domain.OverrideFor(reasons)}
}

func reasonNoEpisode(s domain.Series, p parse.ParsedRelease) *domain.Reason {
	return &domain.Reason{RuleID: "tv.no_episode", Category: domain.ReasonPermanent,
		Message: "release covers no episode of " + s.Title, Values: map[string]any{"parsedTitle": p.Title, "episode": p.Episode}}
}

func reasonMismatch(s domain.Series, p parse.ParsedRelease) *domain.Reason {
	return &domain.Reason{RuleID: "tv.mismatch", Category: domain.ReasonPermanent,
		Message: "release does not match " + s.Title, Values: map[string]any{"parsedTitle": p.Title, "parsedYear": p.Year.Value}}
}
