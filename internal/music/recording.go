package music

import (
	"cmp"
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"unicode"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// Discography returns an artist with every release group from the metadata
// provider, whether or not the artist is in the library.
func (s *Service) Discography(ctx context.Context, artistMBID string) (plugin.ArtistRecord, error) {
	md, err := s.metadata()
	if err != nil {
		return plugin.ArtistRecord{}, err
	}
	return md.Artist(ctx, artistMBID)
}

// RecordingMatch is the release group whose edition holds a track.
type RecordingMatch struct {
	Album plugin.AlbumRecord `json:"album"`
	Track plugin.TrackRecord `json:"track"`
}

// ErrNoRecording says no probed release group of the artist holds the track.
var ErrNoRecording = errors.New("no album, EP or single of this artist holds that track")

// maxRecordingProbes bounds the editions fetched to find a track.
const maxRecordingProbes = 12

// FindRecording finds the release group that holds a track by title: studio
// albums first (oldest first), then EPs and singles, then everything else.
func (s *Service) FindRecording(ctx context.Context, artistMBID, title string) (RecordingMatch, error) {
	rec, err := s.Discography(ctx, artistMBID)
	if err != nil {
		return RecordingMatch{}, err
	}
	md, _ := s.metadata()
	want := normTitle(title)
	for _, al := range probeOrder(rec.Albums, maxRecordingProbes) {
		rels, err := md.Releases(ctx, al.MBID)
		if err != nil {
			return RecordingMatch{}, err
		}
		if t, ok := trackIn(rels, want); ok {
			return RecordingMatch{Album: al, Track: t}, nil
		}
	}
	return RecordingMatch{}, ErrNoRecording
}

// probeOrder ranks release groups for a track search and keeps the first n.
func probeOrder(albums []plugin.AlbumRecord, n int) []plugin.AlbumRecord {
	out := slices.Clone(albums)
	slices.SortStableFunc(out, func(a, b plugin.AlbumRecord) int {
		if d := probeRank(a) - probeRank(b); d != 0 {
			return d
		}
		return cmp.Compare(releaseUnix(a), releaseUnix(b))
	})
	return out[:min(n, len(out))]
}

var typeRank = map[domain.AlbumType]int{domain.AlbumTypeAlbum: 0, domain.AlbumTypeEP: 1, domain.AlbumTypeSingle: 1}

func probeRank(a plugin.AlbumRecord) int {
	rank, ok := typeRank[domain.ParseAlbumType(a.PrimaryType)]
	if !ok {
		rank = 2
	}
	if len(a.SecondaryTypes) > 0 { // compilations, live, remixes
		rank += 3
	}
	return rank
}

// releaseUnix orders by first release; undated groups go last.
func releaseUnix(a plugin.AlbumRecord) int64 {
	if a.FirstRelease == nil {
		return math.MaxInt64
	}
	return a.FirstRelease.Unix()
}

func trackIn(rels []plugin.ReleaseRecord, want string) (plugin.TrackRecord, bool) {
	for _, r := range rels {
		for _, m := range r.Media {
			for _, t := range m.Tracks {
				if normTitle(t.Title) == want {
					return t, true
				}
			}
		}
	}
	return plugin.TrackRecord{}, false
}

// normTitle compares titles by letters and digits only.
func normTitle(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
