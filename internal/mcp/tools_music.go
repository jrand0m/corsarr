package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// ── find_media for music ─────────────────────────────────────────────────────

func (s *Service) findMusicMedia(ctx context.Context, in findArgs) ([]Candidate, error) {
	switch in.Level {
	case "", "artist":
		return s.findArtists(ctx, in.Query)
	case "album":
		return s.findAlbums(ctx, in)
	case "track":
		return s.findTrack(ctx, in)
	}
	return nil, invalidArg("level", "must be artist, album or track")
}

func (s *Service) findArtists(ctx context.Context, query string) ([]Candidate, error) {
	items, err := s.d.Music.Lookup(ctx, query)
	out := make([]Candidate, 0, len(items))
	for _, it := range items {
		out = append(out, Candidate{Kind: domain.KindMusic, Level: "artist", Title: it.Name, Type: it.Type,
			Overview: it.Disambiguation, IDs: map[string]string{"mbid": it.MBID}, InLibrary: it.LibraryID > 0, LibraryID: it.LibraryID})
	}
	return out, err
}

// artistOf resolves the artist argument (a MusicBrainz id or a name; the
// best name match wins).
func (s *Service) artistOf(ctx context.Context, artist string) (music.LookupItem, error) {
	artist = strings.TrimSpace(artist)
	if artist == "" {
		return music.LookupItem{}, invalidArg("artist", "album and track searches need the artist (name or MusicBrainz id)")
	}
	term := artist
	if looksLikeMBID(artist) {
		term = "mbid:" + artist
	}
	items, err := s.d.Music.Lookup(ctx, term)
	if err != nil || len(items) == 0 {
		return music.LookupItem{}, notFoundOr(err, "no artist matches "+artist)
	}
	return items[0], nil
}

func looksLikeMBID(s string) bool { return len(s) == 36 && strings.Count(s, "-") == 4 }

func (s *Service) findAlbums(ctx context.Context, in findArgs) ([]Candidate, error) {
	artist, err := s.artistOf(ctx, in.Artist)
	if err != nil {
		return nil, err
	}
	rec, err := s.d.Music.Discography(ctx, artist.MBID)
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(strings.TrimSpace(in.Query))
	out := []Candidate{}
	for _, al := range rec.Albums {
		if strings.Contains(strings.ToLower(al.Title), want) {
			out = append(out, albumCandidate(artist, al))
		}
	}
	return out, nil
}

func albumCandidate(artist music.LookupItem, al plugin.AlbumRecord) Candidate {
	c := Candidate{Kind: domain.KindMusic, Level: "album", Title: al.Title, Artist: artist.Name, Type: al.PrimaryType,
		IDs: map[string]string{"mbid": al.MBID, "artistMbid": artist.MBID}, InLibrary: artist.LibraryID > 0, LibraryID: artist.LibraryID}
	if al.FirstRelease != nil {
		c.Year = al.FirstRelease.Year()
	}
	return c
}

func (s *Service) findTrack(ctx context.Context, in findArgs) ([]Candidate, error) {
	artist, err := s.artistOf(ctx, in.Artist)
	if err != nil {
		return nil, err
	}
	m, err := s.d.Music.FindRecording(ctx, artist.MBID, in.Query)
	if err != nil {
		return nil, err
	}
	c := albumCandidate(artist, m.Album)
	c.Level, c.Title = "track", m.Track.Title
	c.IDs["recordingMbid"] = m.Track.RecordingMBID
	c.Overview = fmt.Sprintf("track %d of %s %q; request_music with track requests that release", m.Track.Position, m.Album.PrimaryType, m.Album.Title)
	return []Candidate{c}, nil
}

// ── request_music ────────────────────────────────────────────────────────────

type musicRequest struct {
	ArtistMBID string `json:"artistMbid"`
	AlbumMBID  string `json:"albumMbid"`
	Track      string `json:"track"`
	Monitor    string `json:"monitor"`
	ProfileKey string `json:"profileKey"`
	RootFolder string `json:"rootFolder"`
	Search     *bool  `json:"search"`
}

var artistMonitors = map[string]domain.AlbumMonitor{
	"": domain.AlbumMonitorAll, "all": domain.AlbumMonitorAll, "future": domain.AlbumMonitorFuture, "none": domain.AlbumMonitorNone,
}

func (s *Service) requestMusic(ctx context.Context, in musicRequest) (Result, error) {
	if err := s.usable(domain.KindMusic); err != nil {
		return Result{}, err
	}
	if in.ArtistMBID == "" {
		return Result{}, invalidArg("artistMbid", "is required")
	}
	if in.Track != "" {
		return s.requestTrack(ctx, in)
	}
	if in.AlbumMBID != "" {
		return s.requestAlbum(ctx, in)
	}
	return s.requestArtist(ctx, in)
}

func (s *Service) requestArtist(ctx context.Context, in musicRequest) (Result, error) {
	mode, ok := artistMonitors[in.Monitor]
	if !ok {
		return Result{}, invalidArg("monitor", "must be all, future or none")
	}
	a, created, err := s.d.Music.Add(ctx, music.AddRequest{MBID: in.ArtistMBID, Monitor: string(mode), ProfileKey: in.ProfileKey,
		RootFolder: in.RootFolder, SearchNow: wantSearch(in.Search)})
	if err != nil {
		return Result{}, err
	}
	return s.requested(ctx, domain.KindMusic, target{ID: a.ID}, created)
}

// requestAlbum adds the artist with nothing monitored when absent, then
// monitors and searches the one release group.
func (s *Service) requestAlbum(ctx context.Context, in musicRequest) (Result, error) {
	a, _, err := s.d.Music.Add(ctx, music.AddRequest{MBID: in.ArtistMBID, Monitor: string(domain.AlbumMonitorNone),
		MonitorNew: string(domain.MonitorNewItemsNone), ProfileKey: in.ProfileKey, RootFolder: in.RootFolder})
	if err != nil {
		return Result{}, err
	}
	al, err := s.albumByMBID(ctx, a.ID, in.AlbumMBID)
	if err != nil {
		return Result{}, err
	}
	created := !al.Monitored
	if _, err := s.d.Music.SetAlbumsMonitored(ctx, []int64{al.ID}, true); err != nil {
		return Result{}, err
	}
	if wantSearch(in.Search) {
		s.d.Go(func(ctx context.Context) {
			_, err := s.d.MusicSearch.SearchAlbum(ctx, al.ID, domain.OriginAutomatic)
			s.logErr("album search", err)
		})
	}
	return s.requested(ctx, domain.KindMusic, target{AlbumID: al.ID}, created)
}

func (s *Service) albumByMBID(ctx context.Context, artistID int64, mbid string) (domain.Album, error) {
	albums, err := s.d.Music.Albums(ctx, artistID)
	if err != nil {
		return domain.Album{}, err
	}
	for _, al := range albums {
		if al.MBID == mbid {
			return al, nil
		}
	}
	return domain.Album{}, invalidArg("albumMbid", "the artist has no release group "+mbid)
}

// requestTrack requests the album, EP or single that holds the track. The
// direct music sources offer whole releases only, so a track found on none
// is refused with the reason.
func (s *Service) requestTrack(ctx context.Context, in musicRequest) (Result, error) {
	m, err := s.d.Music.FindRecording(ctx, in.ArtistMBID, in.Track)
	if errors.Is(err, music.ErrNoRecording) {
		return Result{}, fmt.Errorf("%w; single-track downloads are not supported, so nothing was requested", err)
	}
	if err != nil {
		return Result{}, err
	}
	in.AlbumMBID = m.Album.MBID
	res, err := s.requestAlbum(ctx, in)
	if err == nil {
		res.Text = fmt.Sprintf("%q is on %s %q; %s", m.Track.Title, m.Album.PrimaryType, m.Album.Title, res.Text)
	}
	return res, err
}

func (s *Service) setArtistMonitored(ctx context.Context, id int64, on bool) error {
	_, err := s.d.Music.Update(ctx, id, music.Patch{Monitored: &on})
	return err
}
