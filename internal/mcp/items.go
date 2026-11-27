package mcp

import (
	"context"
	"errors"

	"github.com/jrand0m/corsarr/internal/domain"
)

// kindOf validates a library kind argument.
func kindOf(s string) (domain.MediaKind, error) {
	switch k := domain.MediaKind(s); k {
	case domain.KindMovie, domain.KindTV, domain.KindMusic:
		return k, nil
	case domain.KindBook:
		return "", unsupported("books are not supported by this build yet")
	}
	return "", invalidArg("kind", "must be movie, tv or music")
}

// usable refuses a disabled kind with its mode (reads and writes alike, as
// the native API does); shadow and active pass.
func (s *Service) usable(k domain.MediaKind) error {
	if mode := s.d.Modes.Of(k); mode == domain.ModeDisabled {
		return domain.ModeError{Kind: k, Mode: mode}
	}
	return nil
}

// kindArg validates the kind and that it is usable.
func (s *Service) kindArg(kind string) (domain.MediaKind, error) {
	k, err := kindOf(kind)
	if err != nil {
		return "", err
	}
	return k, s.usable(k)
}

// target addresses one library item by library id or external id.
type target struct {
	Kind    string `json:"kind"`
	ID      int64  `json:"id"`
	AlbumID int64  `json:"albumId"`
	TMDBID  int64  `json:"tmdbId"`
	TVDBID  int64  `json:"tvdbId"`
	IMDBID  string `json:"imdbId"`
	MBID    string `json:"mbid"`
}

func targetProps() map[string]schema {
	return map[string]schema{
		"kind":    kindEnum("Media kind."),
		"id":      integer("Library id: movie, series or artist."),
		"albumId": integer("Library id of a music album (instead of id)."),
		"tmdbId":  integer("TMDB id (movie or tv) when the library id is unknown."),
		"tvdbId":  integer("TVDB id (tv) when the library id is unknown."),
		"imdbId":  str("IMDb id, e.g. tt0111161 (movie or tv)."),
		"mbid":    str("MusicBrainz id of an artist or album (music)."),
	}
}

// errNotInLibrary says the external id names nothing corsarr has.
var errNotInLibrary = errors.New("not in the library: request it first")

// resolve fills t.ID (or t.AlbumID) from the external ids.
func (s *Service) resolve(ctx context.Context, t target) (target, error) {
	if t.ID > 0 || t.AlbumID > 0 {
		return t, nil
	}
	find := map[string]func(context.Context, target) (target, error){
		string(domain.KindMovie): s.findMovie, string(domain.KindTV): s.findSeries, string(domain.KindMusic): s.findMusic,
	}[t.Kind]
	return find(ctx, t)
}

// stop ends an Each walk early once found.
var stop = errors.New("found")

func walkDone(err error) error {
	if errors.Is(err, stop) {
		return nil
	}
	return err
}

func (s *Service) findMovie(ctx context.Context, t target) (target, error) {
	err := s.d.Movies.Each(ctx, func(m domain.Movie) error {
		if (t.TMDBID > 0 && m.TMDBID == t.TMDBID) || (t.IMDBID != "" && m.IMDBID == t.IMDBID) {
			t.ID = m.ID
			return stop
		}
		return nil
	})
	return found(t, walkDone(err))
}

func (s *Service) findSeries(ctx context.Context, t target) (target, error) {
	err := s.d.TV.EachSeries(ctx, func(ser domain.Series) error {
		if (t.TMDBID > 0 && ser.TMDBID == t.TMDBID) || (t.TVDBID > 0 && ser.TVDBID == t.TVDBID) || (t.IMDBID != "" && ser.IMDBID == t.IMDBID) {
			t.ID = ser.ID
			return stop
		}
		return nil
	})
	return found(t, walkDone(err))
}

// findMusic matches an artist mbid, else an album (release group) mbid.
func (s *Service) findMusic(ctx context.Context, t target) (target, error) {
	if t.MBID == "" {
		return t, invalidArg("mbid", "give id, albumId or mbid")
	}
	err := s.d.Music.EachArtist(ctx, func(a domain.Artist) error {
		if a.MBID == t.MBID {
			t.ID = a.ID
			return stop
		}
		return s.matchAlbum(ctx, a.ID, &t)
	})
	return found(t, walkDone(err))
}

func (s *Service) matchAlbum(ctx context.Context, artistID int64, t *target) error {
	albums, err := s.d.Music.Albums(ctx, artistID)
	if err != nil {
		return err
	}
	for _, al := range albums {
		if al.MBID == t.MBID {
			t.AlbumID = al.ID
			return stop
		}
	}
	return nil
}

func found(t target, err error) (target, error) {
	if err == nil && t.ID == 0 && t.AlbumID == 0 {
		err = errNotInLibrary
	}
	return t, err
}
