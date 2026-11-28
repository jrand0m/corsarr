package music

import (
	"context"
	"errors"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
)

// Album statuses shown per album.
const (
	StatusDownloaded  = "downloaded"
	StatusPartial     = "partial"
	StatusMissing     = "missing"
	StatusNotReleased = "notReleased"
	StatusUnmonitored = "unmonitored"
)

// FileView is a track's library file.
type FileView struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Quality string `json:"quality"`
}

// TrackView is a track with its file.
type TrackView struct {
	domain.Track
	File *FileView `json:"file,omitempty"`
}

// AlbumView is an album with its status and file counts.
type AlbumView struct {
	domain.Album
	Status     string `json:"status"`
	InProfile  bool   `json:"inProfile"`
	FileCount  int    `json:"fileCount"`
	SizeOnDisk int64  `json:"sizeOnDisk"`
	Quality    string `json:"quality,omitempty"` // the lowest quality among its files
	CutoffMet  bool   `json:"cutoffMet"`
	CoverURL   string `json:"coverUrl,omitempty"`
}

// Stats summarise an artist for the list.
type Stats struct {
	AlbumCount      int        `json:"albumCount"` // inside the metadata profile
	TotalAlbums     int        `json:"totalAlbums"`
	MonitoredAlbums int        `json:"monitoredAlbums"`
	AlbumsWithFiles int        `json:"albumsWithFiles"`
	TrackFileCount  int        `json:"trackFileCount"`
	SizeOnDisk      int64      `json:"sizeOnDisk"`
	NextRelease     *time.Time `json:"nextRelease"`
	LastRelease     *time.Time `json:"lastRelease"`
}

// Summary is an artist with its stats.
type Summary struct {
	domain.Artist
	Stats    Stats  `json:"stats"`
	CoverURL string `json:"coverUrl,omitempty"` // the cover of the newest album
}

// coverArchive serves release-group front covers by MusicBrainz id, keyless.
const coverArchive = "https://coverartarchive.org/release-group/"

// CoverURL is the provider address of a release group's front cover.
func CoverURL(mbid string) string {
	if mbid == "" {
		return ""
	}
	return coverArchive + mbid + "/front-500"
}

// ArtistCover picks the cover of the newest album of primary type album;
// undated albums count as oldest (pure).
func ArtistCover(albums []domain.Album) string {
	var best *domain.Album
	for i := range albums {
		a := &albums[i]
		if a.PrimaryType != domain.AlbumTypeAlbum || a.MBID == "" {
			continue
		}
		if best == nil || newer(a.ReleaseDate, best.ReleaseDate) {
			best = a
		}
	}
	if best == nil {
		return ""
	}
	return CoverURL(best.MBID)
}

func newer(a, b *time.Time) bool { return a != nil && (b == nil || a.After(*b)) }

// Detail is the artist page: the artist and every album.
type Detail struct {
	Artist Summary     `json:"artist"`
	Albums []AlbumView `json:"albums"`
}

// ArtistRef is the short artist shape attached to album rows.
type ArtistRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// RefOf shortens an artist.
func RefOf(a domain.Artist) ArtistRef { return ArtistRef{ID: a.ID, Name: a.Name} }

// AlbumDetail is the album page: the album, its artist and its tracks.
type AlbumDetail struct {
	Album  AlbumView   `json:"album"`
	Artist ArtistRef   `json:"artist"`
	Tracks []TrackView `json:"tracks"`
}

// Status is an album's status word from how many of its tracks have files.
func Status(a domain.Album, withFiles int, now time.Time) string {
	switch {
	case a.TrackCount > 0 && withFiles >= a.TrackCount:
		return StatusDownloaded
	case withFiles > 0:
		return StatusPartial
	case !a.Monitored:
		return StatusUnmonitored
	case !IsReleased(a, now):
		return StatusNotReleased
	}
	return StatusMissing
}

// Current is what the album has on disk as the decision engine sees it:
// the lowest-ranked quality among its track files (nil when none).
func Current(p decide.QualityProfile, files []domain.LibraryFile) *decide.CurrentFile {
	var cur *decide.CurrentFile
	low := 0
	for _, f := range files {
		rank, ok := p.Rank(decide.QualityID(f.Quality))
		if !ok {
			rank = -1
		}
		if cur == nil || rank < low {
			low = rank
			cur = &decide.CurrentFile{Quality: decide.QualityID(f.Quality), FormatScore: f.FormatScore, Group: f.Group, Revision: f.Revision}
		}
	}
	return cur
}

// albumView builds one album's view from its tracks' files (pure).
func albumView(ar domain.Artist, al domain.Album, files []domain.LibraryFile, now time.Time) AlbumView {
	profile, _ := ProfileByKey(ar.ProfileKey)
	meta, _ := domain.MetadataProfileByKey(ar.MetadataProfile)
	v := AlbumView{Album: al, InProfile: InProfile(meta, al), FileCount: len(files), Status: Status(al, len(files), now),
		CoverURL: CoverURL(al.MBID)}
	for _, f := range files {
		v.SizeOnDisk += f.Size
	}
	if cur := Current(profile, files); cur != nil {
		v.Quality = qualityName(string(cur.Quality))
		v.CutoffMet = !belowCutoff(profile, *cur)
	}
	return v
}

func qualityName(id string) string {
	if q, ok := decide.LookupQuality(decide.QualityID(id)); ok {
		return q.Name
	}
	return id
}

func belowCutoff(p decide.QualityProfile, cur decide.CurrentFile) bool {
	cut, _ := p.Rank(p.Spec().Cutoff)
	rank, ok := p.Rank(cur.Quality)
	return !ok || rank < cut || cur.FormatScore < p.Spec().CutoffFormatScore
}

// count folds one album into the artist stats.
func count(st *Stats, v AlbumView, now time.Time) {
	st.TotalAlbums++
	st.AlbumCount += one(v.InProfile)
	st.MonitoredAlbums += one(v.Monitored)
	st.AlbumsWithFiles += one(v.FileCount > 0)
	st.TrackFileCount += v.FileCount
	st.SizeOnDisk += v.SizeOnDisk
	releases(st, v.ReleaseDate, now)
}

func one(b bool) int {
	if b {
		return 1
	}
	return 0
}

// releases keeps the next upcoming and the latest past release date.
func releases(st *Stats, d *time.Time, now time.Time) {
	switch {
	case d == nil:
	case d.After(now) && (st.NextRelease == nil || d.Before(*st.NextRelease)):
		st.NextRelease = d
	case !d.After(now) && (st.LastRelease == nil || d.After(*st.LastRelease)):
		st.LastRelease = d
	}
}

// trackFiles loads an album's tracks and the library files they point at.
func (s *Service) trackFiles(ctx context.Context, albumID int64) ([]domain.Track, map[int64]domain.LibraryFile, error) {
	tracks, err := s.d.Repo.Tracks(ctx, albumID)
	files := map[int64]domain.LibraryFile{}
	for _, t := range tracks {
		if t.FileID == 0 {
			continue
		}
		if f, ok, ferr := s.d.Files.File(ctx, t.FileID); ferr == nil && ok {
			files[t.ID] = f
		}
	}
	return tracks, files, err
}

// AlbumFiles are the library files of an album's tracks.
func (s *Service) AlbumFiles(ctx context.Context, albumID int64) ([]domain.LibraryFile, error) {
	_, files, err := s.trackFiles(ctx, albumID)
	out := make([]domain.LibraryFile, 0, len(files))
	for _, f := range files {
		out = append(out, f)
	}
	return out, err
}

// Detail loads the artist page.
func (s *Service) Detail(ctx context.Context, id int64) (Detail, error) {
	ar, err := s.Artist(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	return s.detailOf(ctx, ar)
}

func (s *Service) detailOf(ctx context.Context, ar domain.Artist) (Detail, error) {
	albums, err := s.d.Repo.Albums(ctx, ar.ID)
	if err != nil {
		return Detail{}, err
	}
	now := s.d.Now()
	d := Detail{Artist: Summary{Artist: ar, CoverURL: ArtistCover(albums)}, Albums: make([]AlbumView, len(albums))}
	for i, al := range albums {
		files, err := s.AlbumFiles(ctx, al.ID)
		if err != nil {
			return Detail{}, err
		}
		d.Albums[i] = albumView(ar, al, files, now)
		count(&d.Artist.Stats, d.Albums[i], now)
	}
	return d, nil
}

// List pages artists with their stats.
func (s *Service) List(ctx context.Context, cursor int64, limit int) ([]Summary, int64, error) {
	list, next, err := s.d.Repo.ListArtists(ctx, cursor, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Summary, 0, len(list))
	for _, ar := range list {
		d, err := s.detailOf(ctx, ar)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d.Artist)
	}
	return out, next, nil
}

// AlbumPage loads the album page, fetching its edition when it has none.
func (s *Service) AlbumPage(ctx context.Context, id int64) (AlbumDetail, error) {
	al, _, err := s.EnsureEdition(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return AlbumDetail{}, err
	}
	s.logErr("fetch edition", err) // the page still renders from what is stored
	ar, err := s.Artist(ctx, al.ArtistID)
	if err != nil {
		return AlbumDetail{}, err
	}
	tracks, files, err := s.trackFiles(ctx, id)
	if err != nil {
		return AlbumDetail{}, err
	}
	views := make([]TrackView, len(tracks))
	list := make([]domain.LibraryFile, 0, len(files))
	for i, t := range tracks {
		views[i] = TrackView{Track: t}
		if f, ok := files[t.ID]; ok {
			views[i].File = &FileView{Path: f.Path, Size: f.Size, Quality: qualityName(f.Quality)}
			list = append(list, f)
		}
	}
	return AlbumDetail{Album: albumView(ar, al, list, s.d.Now()), Artist: RefOf(ar), Tracks: views}, nil
}
