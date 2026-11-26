package migrate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/music"
)

// Pure mappings from source concepts onto corsarr's model.

// qualityID maps a source quality name onto the ladder; exact=false means
// the name has no rung and the file is recorded as unknown.
func qualityID(name string) (decide.QualityID, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if base, ok := strings.CutSuffix(n, " remux"); ok {
		n = "remux-" + base[strings.LastIndex(base, "-")+1:]
	}
	if _, ok := decide.LookupQuality(decide.QualityID(n)); ok {
		return decide.QualityID(n), true
	}
	return "unknown", false
}

// shippedProfiles lists corsarr's profile keys for a kind.
func shippedProfiles(k domain.MediaKind) map[string]bool {
	out := map[string]bool{}
	for _, p := range decide.DefaultProfiles(k) {
		out[movies.ProfileKey(p.Spec().Name)] = true
	}
	return out
}

// ProfileMapping is how one source quality profile lands in corsarr.
type ProfileMapping struct {
	Key    string
	Exact  bool
	Reason string
}

// mapProfile picks the shipped profile with the same name, else the
// closest one by the source cutoff.
func mapProfile(p SrcProfile, k domain.MediaKind) ProfileMapping {
	shipped := shippedProfiles(k)
	key := movies.ProfileKey(p.Name)
	notes := profileNotes(p, k)
	if shipped[key] {
		return ProfileMapping{Key: key, Exact: notes == "", Reason: "same as the built-in profile " + key + notes}
	}
	closest := closestProfile(p, k)
	return ProfileMapping{Key: closest, Reason: fmt.Sprintf("custom profiles are not supported yet; items use the built-in %s (closest by cutoff %q)%s",
		closest, p.Cutoff, notes)}
}

// profileNotes lists what a profile loses on the way in.
func profileNotes(p SrcProfile, k domain.MediaKind) string {
	out := ""
	if p.Scored > 0 {
		out = fmt.Sprintf("; %d custom format scores are not carried over", p.Scored)
	}
	if k != domain.KindMusic {
		return out
	}
	if below := belowMusicFloor(p.Allowed); len(below) > 0 {
		out += "; allowed below corsarr's music quality floor and never grabbed: " + strings.Join(below, ", ")
	}
	return out
}

func closestProfile(p SrcProfile, k domain.MediaKind) string {
	if k == domain.KindMusic {
		q, _ := audioQualityID(p.Cutoff)
		if t, _ := decide.AudioTierOf(q); t == decide.AudioLossless || t == decide.AudioHiRes {
			return "lossless"
		}
		return music.DefaultProfileKey
	}
	if strings.Contains(p.Cutoff, "2160") {
		return "ultrahd"
	}
	return movies.DefaultProfileKey
}

// profileKey maps an item's profile id, defaulting when the id is unknown.
func profileKey(profiles map[int]SrcProfile, id int, k domain.MediaKind) string {
	if p, ok := profiles[id]; ok {
		return mapProfile(p, k).Key
	}
	if k == domain.KindMusic {
		return music.DefaultProfileKey
	}
	return movies.DefaultProfileKey
}

// tagLabels resolves tag ids to labels, sorted.
func tagLabels(tags map[int]string, ids []int) []string {
	out := []string{}
	for _, id := range ids {
		if l, ok := tags[id]; ok {
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out
}

// seriesTarget maps a source series onto corsarr's series, seasons and episodes.
func seriesTarget(s SrcSeries, snap Snapshot, rw Rewrite) (domain.Series, []domain.Season, []domain.Episode) {
	ser := domain.Series{TMDBID: s.TMDB, TVDBID: s.TVDB, IMDBID: s.IMDB, Title: s.Title, SortTitle: sortTitle(s.Title), Year: s.Year,
		Status: s.Status, Network: s.Network, RuntimeMin: s.Runtime, RootFolder: rw.Apply(s.Root), Path: rw.Apply(s.Path),
		ProfileKey: profileKey(snap.Profiles, s.ProfileID, domain.KindTV), Tags: tagLabels(snap.Tags, s.Tags), Type: s.Type,
		SeasonFolders: s.SeasonFolders, MonitorNew: s.MonitorNew, Monitored: s.Monitored}
	eps := make([]domain.Episode, 0, len(s.Episodes))
	for _, e := range s.Episodes {
		eps = append(eps, domain.Episode{Season: e.Season, Number: e.Number, Absolute: e.Absolute, Title: e.Title, AirAt: e.AirAt,
			RuntimeMin: s.Runtime, Monitored: e.Monitored, InMetadata: true})
	}
	return ser, append([]domain.Season(nil), s.Seasons...), eps
}

func sortTitle(t string) string {
	l := strings.ToLower(t)
	for _, a := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(l, a) {
			return strings.TrimPrefix(l, a)
		}
	}
	return l
}

// SeriesTargetHash fingerprints the settings and monitoring a migration
// writes for a series; it tells a corsarr edit from an untouched object.
func SeriesTargetHash(s domain.Series, seasons []domain.Season, eps []domain.Episode) string {
	mon := make([]string, 0, len(seasons)+len(eps))
	for _, se := range seasons {
		mon = append(mon, fmt.Sprintf("s%d=%t", se.Number, se.Monitored))
	}
	for _, e := range eps {
		mon = append(mon, fmt.Sprintf("e%d.%d=%t", e.Season, e.Number, e.Monitored))
	}
	sort.Strings(mon)
	return hashOf([]any{s.Monitored, s.ProfileKey, strings.Join(s.Tags, ","), s.Type, s.SeasonFolders, s.MonitorNew,
		s.RootFolder, s.Path, mon})
}

// minAvailability maps the source value; exact=false when it was folded.
func minAvailability(raw string) (domain.MinAvailability, bool) {
	switch raw {
	case "tba":
		return domain.AvailAnnounced, false
	case "preDB":
		return domain.AvailReleased, false
	}
	a, err := domain.ParseMinAvailability(raw)
	if err != nil {
		return domain.AvailReleased, false
	}
	return a, true
}

// movieTarget maps a source movie onto corsarr's movie.
func movieTarget(m SrcMovie, snap Snapshot, rw Rewrite) (domain.Movie, []string) {
	avail, exact := minAvailability(m.MinAvailability)
	var notes []string
	if !exact {
		notes = append(notes, fmt.Sprintf("minimum availability %q became %s", m.MinAvailability, avail))
	}
	if len(m.Tags) > 0 {
		notes = append(notes, "movie tags are not kept yet")
	}
	return domain.Movie{TMDBID: m.TMDB, IMDBID: m.IMDB, Title: m.Title, OriginalTitle: m.Original, Year: m.Year, RuntimeMin: m.Runtime,
		InCinemas: m.InCinemas, Digital: m.Digital, Physical: m.Physical, Monitored: m.Monitored, MinAvailability: avail,
		ProfileKey: profileKey(snap.Profiles, m.ProfileID, domain.KindMovie), RootFolder: rw.Apply(m.Root), Path: rw.Apply(m.Path)}, notes
}

// MovieTargetHash fingerprints the settings a migration writes for a movie.
func MovieTargetHash(m domain.Movie) string {
	return hashOf([]any{m.Monitored, m.MinAvailability, m.ProfileKey, m.RootFolder, m.Path})
}

// libraryFile maps a source file onto a library file at its (rewritten)
// path, with the quality on the kind's ladder.
func libraryFile(f SrcFile, rw Rewrite, k domain.MediaKind) (domain.LibraryFile, bool) {
	q, exact := qualityID(f.Quality)
	if k == domain.KindMusic {
		q, exact = audioQualityID(f.Quality)
	}
	return domain.LibraryFile{Path: rw.Apply(f.Path), Size: f.Size, Quality: string(q), Group: f.Group, Edition: f.Edition,
		Revision: 1, AddedAt: f.Added}, exact
}
