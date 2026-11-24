package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Item reference variants of the music kind: the album (release group) is the
// wanted unit and carries no variant; artists and track files have their own.
const (
	VariantArtist = "artist"
	VariantTrack  = "track"
)

// AlbumType is the primary type of a release group.
type AlbumType string

const (
	AlbumTypeAlbum     AlbumType = "album"
	AlbumTypeEP        AlbumType = "ep"
	AlbumTypeSingle    AlbumType = "single"
	AlbumTypeBroadcast AlbumType = "broadcast"
	AlbumTypeOther     AlbumType = "other"
)

// ParseAlbumType reads a provider's primary type; anything unknown is "other".
func ParseAlbumType(s string) AlbumType {
	switch t := AlbumType(strings.ToLower(s)); t {
	case AlbumTypeAlbum, AlbumTypeEP, AlbumTypeSingle, AlbumTypeBroadcast:
		return t
	}
	return AlbumTypeOther
}

// MonitorNewItems says which release groups that appear on a refresh are
// monitored: all of them, those released after the artist was added, or none.
type MonitorNewItems string

const (
	MonitorNewItemsAll  MonitorNewItems = "all"
	MonitorNewItemsNew  MonitorNewItems = "new"
	MonitorNewItemsNone MonitorNewItems = "none"
)

// ParseMonitorNewItems validates outside input; "" means all.
func ParseMonitorNewItems(s string) (MonitorNewItems, error) {
	switch m := MonitorNewItems(s); m {
	case "":
		return MonitorNewItemsAll, nil
	case MonitorNewItemsAll, MonitorNewItemsNew, MonitorNewItemsNone:
		return m, nil
	}
	return "", fmt.Errorf("monitor new items %q must be all, new or none", s)
}

// AlbumMonitor is the one-time monitoring choice when an artist is added.
type AlbumMonitor string

const (
	AlbumMonitorAll      AlbumMonitor = "all"
	AlbumMonitorMissing  AlbumMonitor = "missing"
	AlbumMonitorExisting AlbumMonitor = "existing"
	AlbumMonitorFuture   AlbumMonitor = "future"
	AlbumMonitorFirst    AlbumMonitor = "first"
	AlbumMonitorLatest   AlbumMonitor = "latest"
	AlbumMonitorNone     AlbumMonitor = "none"
)

// ParseAlbumMonitor validates outside input; "" means all.
func ParseAlbumMonitor(s string) (AlbumMonitor, error) {
	switch m := AlbumMonitor(s); m {
	case "":
		return AlbumMonitorAll, nil
	case AlbumMonitorAll, AlbumMonitorMissing, AlbumMonitorExisting, AlbumMonitorFuture, AlbumMonitorFirst,
		AlbumMonitorLatest, AlbumMonitorNone:
		return m, nil
	}
	return "", fmt.Errorf("unknown album monitoring %q", s)
}

// MetadataProfile decides which release groups automation cares about: it
// governs auto-monitoring and searching only; every release group of a
// library artist stays visible, monitorable and importable.
type MetadataProfile struct {
	Key       string      `json:"key"`
	Name      string      `json:"name"`
	Primary   []AlbumType `json:"primaryTypes"`
	Secondary []string    `json:"secondaryTypes"` // allowed secondary types (lower case)
}

// MetadataProfiles are the shipped metadata profiles.
var MetadataProfiles = []MetadataProfile{
	{Key: "standard", Name: "Standard", Primary: []AlbumType{AlbumTypeAlbum}, Secondary: []string{}},
	{Key: "standardsingles", Name: "Standard + Singles", Primary: []AlbumType{AlbumTypeAlbum, AlbumTypeEP, AlbumTypeSingle},
		Secondary: []string{}},
	{Key: "none", Name: "None", Primary: []AlbumType{}, Secondary: []string{}},
}

// DefaultMetadataProfile is the profile an artist gets when none is chosen.
const DefaultMetadataProfile = "standard"

// MetadataProfileByKey finds a shipped metadata profile.
func MetadataProfileByKey(key string) (MetadataProfile, bool) {
	for _, p := range MetadataProfiles {
		if p.Key == key {
			return p, true
		}
	}
	return MetadataProfile{}, false
}

// Allows reports whether a release group is inside the profile: its primary
// type is allowed and every secondary type it has is allowed.
func (p MetadataProfile) Allows(primary AlbumType, secondary []string) bool {
	if !slices.Contains(p.Primary, primary) {
		return false
	}
	for _, s := range secondary {
		if !slices.Contains(p.Secondary, strings.ToLower(s)) {
			return false
		}
	}
	return true
}

// Artist is the music library's top level.
type Artist struct {
	ID              int64           `json:"id"`
	MBID            string          `json:"mbid"`
	Name            string          `json:"name"`
	SortName        string          `json:"sortName"`
	Disambiguation  string          `json:"disambiguation,omitempty"`
	Type            string          `json:"type,omitempty"`
	Country         string          `json:"country,omitempty"`
	RootFolder      string          `json:"rootFolder"`
	Path            string          `json:"path"`
	ProfileKey      string          `json:"profileKey"`
	MetadataProfile string          `json:"metadataProfile"`
	MonitorNew      MonitorNewItems `json:"monitorNew"`
	Monitored       bool            `json:"monitored"`
	Tags            []string        `json:"tags"`
	AddedAt         time.Time       `json:"addedAt"`
}

// Ref is the artist-level item reference.
func (a Artist) Ref() ItemRef { return ItemRef{Kind: KindMusic, ID: a.ID, Variant: VariantArtist} }

// EventItem snapshots the artist for an event envelope.
func (a Artist) EventItem() EventItem { return EventItem{Ref: a.Ref(), Title: a.Name, Tags: a.Tags} }

// PinReason says why a release group stays monitored whatever the profile.
type PinReason string

const (
	PinNone   PinReason = ""
	PinOwner  PinReason = "owner"
	PinList   PinReason = "list"
	PinImport PinReason = "import"
)

// Album is a release group: the music wanted unit. Its tracks come from the
// selected edition (release).
type Album struct {
	ID             int64      `json:"id"`
	ArtistID       int64      `json:"artistId"`
	MBID           string     `json:"mbid"`
	Title          string     `json:"title"`
	PrimaryType    AlbumType  `json:"primaryType"`
	SecondaryTypes []string   `json:"secondaryTypes"`
	ReleaseDate    *time.Time `json:"releaseDate"`
	Monitored      bool       `json:"monitored"`
	PinReason      PinReason  `json:"pinReason,omitempty"`
	EditionMBID    string     `json:"editionMbid,omitempty"`
	EditionLocked  bool       `json:"editionLocked"`
	MediumCount    int        `json:"mediumCount"`
	TrackCount     int        `json:"trackCount"`
	DurationSec    int        `json:"durationSec"`
}

// Ref is the album's item reference.
func (a Album) Ref() ItemRef { return ItemRef{Kind: KindMusic, ID: a.ID} }

// Year is the year of first release (0 when unknown).
func (a Album) Year() int {
	if a.ReleaseDate == nil {
		return 0
	}
	return a.ReleaseDate.Year()
}

// Pinned reports whether profile changes leave the album alone.
func (a Album) Pinned() bool { return a.PinReason != PinNone }

// EventItem snapshots the album for an event envelope, named after its artist.
func (a Album) EventItem(artist string) EventItem {
	title := a.Title
	if artist != "" {
		title = artist + " - " + a.Title
	}
	return EventItem{Ref: a.Ref(), Title: title}
}

// Track is one track of an album's selected edition.
type Track struct {
	ID           int64  `json:"id"`
	AlbumID      int64  `json:"albumId"`
	RecordingID  string `json:"recordingMbid,omitempty"`
	Medium       int    `json:"medium"`
	MediumFormat string `json:"mediumFormat,omitempty"`
	Number       int    `json:"number"`
	Title        string `json:"title"`
	DurationSec  int    `json:"durationSec"`
	FileID       int64  `json:"fileId,omitempty"` // 0 = no file
}

// Ref is the track's reference; library files of music point at tracks.
func (t Track) Ref() ItemRef { return ItemRef{Kind: KindMusic, ID: t.ID, Variant: VariantTrack} }
