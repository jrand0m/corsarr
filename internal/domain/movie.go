package domain

import (
	"fmt"
	"time"
)

// MinAvailability is the earliest point at which a movie may be searched and
// grabbed.
type MinAvailability string

const (
	AvailAnnounced MinAvailability = "announced"
	AvailInCinemas MinAvailability = "inCinemas"
	AvailReleased  MinAvailability = "released"
)

// ParseMinAvailability validates outside input; "" means the default "released".
func ParseMinAvailability(s string) (MinAvailability, error) {
	switch a := MinAvailability(s); a {
	case "":
		return AvailReleased, nil
	case AvailAnnounced, AvailInCinemas, AvailReleased:
		return a, nil
	}
	return "", fmt.Errorf("minimum availability %q must be announced, inCinemas or released", s)
}

// Movie is the movie item (the MVP subset).
type Movie struct {
	ID              int64           `json:"id"`
	TMDBID          int64           `json:"tmdbId"`
	IMDBID          string          `json:"imdbId,omitempty"`
	Title           string          `json:"title"`
	OriginalTitle   string          `json:"originalTitle,omitempty"`
	Year            int             `json:"year"`
	AltTitles       []string        `json:"altTitles,omitempty"`
	RuntimeMin      int             `json:"runtimeMin,omitempty"`
	InCinemas       *time.Time      `json:"inCinemas,omitempty"`
	Digital         *time.Time      `json:"digital,omitempty"`
	Physical        *time.Time      `json:"physical,omitempty"`
	Monitored       bool            `json:"monitored"`
	MinAvailability MinAvailability `json:"minAvailability"`
	ProfileKey      string          `json:"profileKey"`
	RootFolder      string          `json:"rootFolder"`
	Path            string          `json:"path"`
	FileID          int64           `json:"fileId,omitempty"` // 0 = no file
	AddedAt         time.Time       `json:"addedAt"`
}

// Ref is the movie's item reference.
func (m Movie) Ref() ItemRef { return ItemRef{Kind: KindMovie, ID: m.ID} }

// EventItem snapshots the movie for an event envelope.
func (m Movie) EventItem() EventItem { return EventItem{Ref: m.Ref(), Title: m.Title} }

// LibraryFile is one imported media file (owned by the importer).
type LibraryFile struct {
	ID          int64     `json:"id"`
	Item        ItemRef   `json:"item"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	Quality     string    `json:"quality"` // quality id on the ladder (decide.QualityID)
	FormatScore int       `json:"formatScore"`
	Group       string    `json:"group,omitempty"`
	Revision    int       `json:"revision"`
	Edition     string    `json:"edition,omitempty"`
	AddedAt     time.Time `json:"addedAt"`
}

// ReleaseIdentity names a release for blocklist and history lookups.
type ReleaseIdentity struct {
	Indexer  ObjectKey `json:"indexer,omitempty"`
	GUID     string    `json:"guid,omitempty"`
	InfoHash string    `json:"infoHash,omitempty"`
	Title    string    `json:"title"`
}

// IdentityOf extracts the identity of a release.
func IdentityOf(r Release) ReleaseIdentity {
	id := ReleaseIdentity{GUID: r.GUID, InfoHash: r.InfoHash, Title: r.Title}
	if len(r.Indexers) > 0 {
		id.Indexer = r.Indexers[0].Key
	}
	return id
}

// BlocklistEntry keeps a release from being grabbed again for an item.
type BlocklistEntry struct {
	ID      int64           `json:"id"`
	Item    ItemRef         `json:"item"`
	Release ReleaseIdentity `json:"release"`
	Source  string          `json:"source"` // manual | downloader | payload | rule | user
	Reason  string          `json:"reason"`
	AddedAt time.Time       `json:"addedAt"`
}
