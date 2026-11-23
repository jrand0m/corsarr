package tv

import (
	"context"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
)

// Episode statuses shown per episode.
const (
	StatusDownloaded  = "downloaded"
	StatusMissing     = "missing"
	StatusNotAired    = "notAired"
	StatusUnmonitored = "unmonitored"
)

// FileView is an episode's library file.
type FileView struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Quality   string `json:"quality"`
	CutoffMet bool   `json:"cutoffMet"`
}

// EpisodeView is an episode with its status and file.
type EpisodeView struct {
	domain.Episode
	Status string    `json:"status"`
	File   *FileView `json:"file,omitempty"`
}

// SeasonView is a season with its counts.
type SeasonView struct {
	Number         int   `json:"number"`
	Monitored      bool  `json:"monitored"`
	EpisodeCount   int   `json:"episodeCount"`
	AiredCount     int   `json:"airedCount"`
	MonitoredCount int   `json:"monitoredCount"`
	FileCount      int   `json:"fileCount"`
	SizeOnDisk     int64 `json:"sizeOnDisk"`
}

// Stats summarise a series for the list.
type Stats struct {
	SeasonCount       int        `json:"seasonCount"`
	EpisodeCount      int        `json:"episodeCount"` // monitored and aired
	EpisodeFileCount  int        `json:"episodeFileCount"`
	TotalEpisodeCount int        `json:"totalEpisodeCount"`
	SizeOnDisk        int64      `json:"sizeOnDisk"`
	NextAiring        *time.Time `json:"nextAiring"`
	PreviousAiring    *time.Time `json:"previousAiring"`
}

// Summary is a series with its stats.
type Summary struct {
	domain.Series
	Stats Stats `json:"stats"`
}

// Detail is the series page: series, seasons and episodes.
type Detail struct {
	Series   Summary       `json:"series"`
	Seasons  []SeasonView  `json:"seasons"`
	Episodes []EpisodeView `json:"episodes"`
}

// Status is an episode's shared status word.
func Status(e domain.Episode, now time.Time) string {
	switch {
	case e.FileID != 0:
		return StatusDownloaded
	case !e.Monitored:
		return StatusUnmonitored
	case !IsAvailable(e, now):
		return StatusNotAired
	}
	return StatusMissing
}

// CutoffMet reports whether a file meets the profile's cutoff.
func CutoffMet(p decide.QualityProfile, f domain.LibraryFile) bool {
	spec := p.Spec()
	cut, _ := p.Rank(spec.Cutoff)
	rank, ok := p.Rank(decide.QualityID(f.Quality))
	return ok && rank >= cut && f.FormatScore >= spec.CutoffFormatScore
}

// Build derives the detail views (pure): files maps file id → file.
func Build(ser domain.Series, seasons []domain.Season, eps []domain.Episode, files map[int64]domain.LibraryFile, now time.Time) Detail {
	profile, _ := ProfileByKey(ser.ProfileKey)
	views := make([]EpisodeView, len(eps))
	bySeason := map[int]*SeasonView{}
	var order []int
	for _, se := range seasons {
		bySeason[se.Number] = &SeasonView{Number: se.Number, Monitored: se.Monitored}
		order = append(order, se.Number)
	}
	stats := Stats{TotalEpisodeCount: len(eps)}
	for i, e := range eps {
		views[i] = EpisodeView{Episode: e, Status: Status(e, now)}
		sv := bySeason[e.Season]
		if sv == nil {
			sv = &SeasonView{Number: e.Season}
			bySeason[e.Season], order = sv, append(order, e.Season)
		}
		if f, ok := files[e.FileID]; ok && e.FileID != 0 {
			views[i].File = fileView(profile, f)
		}
		count(sv, &stats, views[i], now)
	}
	return Detail{Series: Summary{Series: ser, Stats: stats}, Seasons: seasonList(order, bySeason, &stats), Episodes: views}
}

func fileView(p decide.QualityProfile, f domain.LibraryFile) *FileView {
	name := f.Quality
	if q, ok := decide.LookupQuality(decide.QualityID(f.Quality)); ok {
		name = q.Name
	}
	return &FileView{Path: f.Path, Size: f.Size, Quality: name, CutoffMet: CutoffMet(p, f)}
}

func count(sv *SeasonView, st *Stats, v EpisodeView, now time.Time) {
	sv.EpisodeCount++
	aired := IsAvailable(v.Episode, now)
	if aired {
		sv.AiredCount++
	}
	if v.Monitored {
		sv.MonitoredCount++
	}
	if v.Monitored && aired && v.Season > 0 {
		st.EpisodeCount++
	}
	if v.File != nil {
		sv.FileCount++
		sv.SizeOnDisk += v.File.Size
		st.EpisodeFileCount++
		st.SizeOnDisk += v.File.Size
	}
	airing(st, v.Episode, now)
}

func airing(st *Stats, e domain.Episode, now time.Time) {
	if e.AirAt == nil {
		return
	}
	a := *e.AirAt
	switch {
	case a.After(now) && (st.NextAiring == nil || a.Before(*st.NextAiring)):
		st.NextAiring = &a
	case !a.After(now) && (st.PreviousAiring == nil || a.After(*st.PreviousAiring)):
		st.PreviousAiring = &a
	}
}

func seasonList(order []int, by map[int]*SeasonView, st *Stats) []SeasonView {
	out := make([]SeasonView, 0, len(order))
	for _, n := range order {
		out = append(out, *by[n])
		if n > 0 {
			st.SeasonCount++
		}
	}
	return out
}

// Detail loads the series page.
func (s *Service) Detail(ctx context.Context, id int64) (Detail, error) {
	ser, err := s.Series(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	seasons, err := s.d.Repo.Seasons(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	eps, err := s.d.Repo.Episodes(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	return Build(ser, seasons, eps, s.files(ctx, eps), s.d.Now()), nil
}

func (s *Service) files(ctx context.Context, eps []domain.Episode) map[int64]domain.LibraryFile {
	out := map[int64]domain.LibraryFile{}
	for _, e := range eps {
		if e.FileID == 0 {
			continue
		}
		if _, done := out[e.FileID]; done {
			continue
		}
		if f, ok, err := s.d.Files.File(ctx, e.FileID); err == nil && ok {
			out[e.FileID] = f
		}
	}
	return out
}

// List pages series with their stats.
func (s *Service) List(ctx context.Context, cursor int64, limit int) ([]Summary, int64, error) {
	list, next, err := s.d.Repo.ListSeries(ctx, cursor, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Summary, 0, len(list))
	for _, ser := range list {
		d, err := s.detailOf(ctx, ser)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d.Series)
	}
	return out, next, nil
}

func (s *Service) detailOf(ctx context.Context, ser domain.Series) (Detail, error) {
	eps, err := s.d.Repo.Episodes(ctx, ser.ID)
	if err != nil {
		return Detail{}, err
	}
	return Build(ser, nil, eps, s.files(ctx, eps), s.d.Now()), nil
}

// SeriesRef is the short series shape attached to episode rows.
type SeriesRef struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	PosterURL string `json:"posterUrl,omitempty"`
}

// RefOf shortens a series.
func RefOf(s domain.Series) SeriesRef {
	return SeriesRef{ID: s.ID, Title: s.Title, Year: s.Year, PosterURL: s.PosterURL}
}

// CalendarItem is one upcoming or recent episode.
type CalendarItem struct {
	Episode EpisodeView `json:"episode"`
	Series  SeriesRef   `json:"series"`
}

// Upcoming lists the monitored episodes of monitored series airing in
// [from, to), for the calendar.
func (s *Service) Upcoming(ctx context.Context, from, to time.Time) ([]CalendarItem, error) {
	series := map[int64]domain.Series{}
	out := []CalendarItem{}
	now := s.d.Now()
	err := s.d.Repo.AiringBetween(ctx, from, to, func(e domain.Episode) error {
		ser, ok := series[e.SeriesID]
		if !ok {
			got, found, err := s.d.Repo.Series(ctx, e.SeriesID)
			if err != nil || !found {
				return err
			}
			ser, series[e.SeriesID] = got, got
		}
		if ser.Monitored && e.Monitored {
			out = append(out, CalendarItem{Episode: EpisodeView{Episode: e, Status: Status(e, now)}, Series: RefOf(ser)})
		}
		return nil
	})
	return out, err
}
