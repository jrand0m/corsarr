package tv

import (
	"path/filepath"
	"strconv"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/naming"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// ApplyRecord copies a provider record's metadata onto a series.
func ApplyRecord(s *domain.Series, rec plugin.SeriesRecord) {
	s.TMDBID, _ = strconv.ParseInt(rec.IDs["tmdb"], 10, 64)
	s.TVDBID, _ = strconv.ParseInt(rec.IDs["tvdb"], 10, 64)
	s.IMDBID, s.Title, s.SortTitle, s.Year = rec.IDs["imdb"], rec.Title, SortTitle(rec.Title), rec.Year
	s.Status, s.Network, s.Overview, s.RuntimeMin, s.PosterURL = rec.Status, rec.Network, rec.Overview, rec.RuntimeMin, rec.PosterURL
	s.FanartURL = rec.FanartURL
	s.Genres, s.AltTitles = nonNil(rec.Genres), nonNil(rec.AltTitles)
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

// FolderName renders the series folder from the default template.
func FolderName(s domain.Series) string {
	name, _ := naming.Preview(naming.DefaultSeriesFolder, naming.Values{SeriesTitle: s.Title, ReleaseYear: s.Year})
	return name
}

// Episodes turns a record's episodes into library episodes (unmonitored).
func Episodes(rec plugin.SeriesRecord) []domain.Episode {
	out := make([]domain.Episode, len(rec.Episodes))
	for i, r := range rec.Episodes {
		out[i] = fromRecord(r)
	}
	return out
}

func fromRecord(r plugin.EpisodeRecord) domain.Episode {
	return domain.Episode{Season: r.Season, Number: r.Number, Absolute: r.Absolute, Title: r.Title, AirAt: r.AirAt,
		RuntimeMin: r.RuntimeMin, InMetadata: true}
}

type epKey struct{ season, number int }

// Merge is the result of refreshing a series from its provider record.
type Merge struct {
	Seasons  []domain.Season  // new seasons only
	Upserts  []domain.Episode // every episode to write
	Drop     []int64          // removed upstream, no file: deleted
	Added    int
	Orphaned int // removed upstream but kept for their file
}

// MergeRecord reconciles stored episodes with the provider's: existing ones
// take the new title and dates and keep their monitoring; new ones in a
// known season follow the season's flag, in a new season the series'
// monitor-new-seasons setting; specials never start monitored. Episodes
// gone upstream are deleted unless they have a file, which keeps them
// flagged as not in metadata.
func MergeRecord(s domain.Series, seasons []domain.Season, stored []domain.Episode, rec plugin.SeriesRecord) Merge {
	known := map[int]bool{}
	for _, se := range seasons {
		known[se.Number] = se.Monitored
	}
	have := map[epKey]domain.Episode{}
	for _, e := range stored {
		have[epKey{e.Season, e.Number}] = e
	}
	var m Merge
	seen := map[epKey]bool{}
	for _, r := range rec.Episodes {
		k := epKey{r.Season, r.Number}
		seen[k] = true
		m.Upserts = append(m.Upserts, m.merge(s, known, have, r))
	}
	for _, e := range stored {
		if !seen[epKey{e.Season, e.Number}] {
			m.gone(e)
		}
	}
	return m
}

func (m *Merge) merge(s domain.Series, known map[int]bool, have map[epKey]domain.Episode, r plugin.EpisodeRecord) domain.Episode {
	e := fromRecord(r)
	if old, ok := have[epKey{r.Season, r.Number}]; ok {
		e.ID, e.SeriesID, e.Monitored, e.FileID = old.ID, old.SeriesID, old.Monitored, old.FileID
		return e
	}
	m.Added++
	mon, ok := known[r.Season]
	if !ok {
		mon = s.MonitorNew == domain.MonitorNewAll && r.Season > 0
		known[r.Season] = mon
		m.Seasons = append(m.Seasons, domain.Season{SeriesID: s.ID, Number: r.Season, Monitored: mon})
	}
	e.SeriesID, e.Monitored = s.ID, mon && r.Season > 0
	return e
}

func (m *Merge) gone(e domain.Episode) {
	if e.FileID == 0 {
		m.Drop = append(m.Drop, e.ID)
		return
	}
	if e.InMetadata {
		m.Orphaned++
	}
	e.InMetadata = false
	m.Upserts = append(m.Upserts, e)
}

// SeasonFolder is the folder of a season inside the series folder ("" when
// the series keeps every episode in its own folder).
func SeasonFolder(s domain.Series, season int) string {
	switch {
	case !s.SeasonFolders:
		return ""
	case season == 0:
		return naming.DefaultSpecialFolder
	}
	name, _ := naming.Preview(naming.DefaultSeasonFolder, naming.Values{Season: pad(season)})
	return name
}

func pad(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// EpisodeNumbers renders the episode token of a (multi-)episode file: "01"
// or "01-E03" for a run; the episodes are ordered.
func EpisodeNumbers(eps []domain.Episode) string {
	if len(eps) == 1 {
		return pad(eps[0].Number)
	}
	return pad(eps[0].Number) + "-E" + pad(eps[len(eps)-1].Number)
}

// FilePath is where an episode file goes: series folder, season folder and
// the default episode file name.
func FilePath(s domain.Series, eps []domain.Episode, quality, ext string) string {
	title := ""
	if len(eps) == 1 {
		title = eps[0].Title
	}
	name, _ := naming.Options{Extension: ext}.Preview(naming.DefaultEpisodeFile, naming.Values{SeriesTitle: s.Title,
		Season: pad(eps[0].Season), Episode: EpisodeNumbers(eps), EpisodeTitle: title, QualityFull: quality})
	return filepath.Join(s.Path, SeasonFolder(s, eps[0].Season), name)
}
