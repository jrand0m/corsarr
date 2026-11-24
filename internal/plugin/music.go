package plugin

import (
	"context"
	"time"
)

// ── music metadata ───────────────────────────────────────────────────────────

// ArtistRecord is an artist as a music metadata provider reports it; Albums
// is filled by Artist and left empty by SearchArtists.
type ArtistRecord struct {
	MBID           string        `json:"mbid"`
	Name           string        `json:"name"`
	SortName       string        `json:"sortName"`
	Disambiguation string        `json:"disambiguation,omitempty"`
	Type           string        `json:"type,omitempty"`
	Country        string        `json:"country,omitempty"`
	BeginYear      int           `json:"beginYear,omitempty"`
	Albums         []AlbumRecord `json:"albums,omitempty"`
}

// AlbumRecord is one release group of an artist.
type AlbumRecord struct {
	MBID           string     `json:"mbid"`
	Title          string     `json:"title"`
	PrimaryType    string     `json:"primaryType"`
	SecondaryTypes []string   `json:"secondaryTypes,omitempty"`
	FirstRelease   *time.Time `json:"firstRelease,omitempty"`
}

// TrackRecord is one track of a medium.
type TrackRecord struct {
	RecordingMBID string `json:"recordingMbid"`
	Position      int    `json:"position"`
	Title         string `json:"title"`
	LengthSec     int    `json:"lengthSec"`
}

// MediumRecord is one disc (or side set) of an edition.
type MediumRecord struct {
	Position int           `json:"position"`
	Format   string        `json:"format,omitempty"`
	Tracks   []TrackRecord `json:"tracks"`
}

// ReleaseRecord is one edition of a release group with its media.
type ReleaseRecord struct {
	MBID    string         `json:"mbid"`
	Title   string         `json:"title"`
	Status  string         `json:"status,omitempty"`
	Date    *time.Time     `json:"date,omitempty"`
	Country string         `json:"country,omitempty"`
	Media   []MediumRecord `json:"media"`
}

// TrackCount is the number of tracks over every medium.
func (r ReleaseRecord) TrackCount() int {
	n := 0
	for _, m := range r.Media {
		n += len(m.Tracks)
	}
	return n
}

// MusicMetadata serves music lookups (MusicBrainz first).
type MusicMetadata interface {
	Tester
	SearchArtists(ctx context.Context, query string) ([]ArtistRecord, error)
	// Artist returns the artist with every release group.
	Artist(ctx context.Context, mbid string) (ArtistRecord, error)
	// Releases returns the editions of a release group with their tracks.
	Releases(ctx context.Context, releaseGroupMBID string) ([]ReleaseRecord, error)
}
