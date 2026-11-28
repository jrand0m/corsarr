// Package music is the music media kind: artists and their release groups
// (albums) kept from the music metadata plugin, metadata profiles that steer
// automatic monitoring without hiding anything, edition choice, and the
// kind's adapter for searching, deciding, matching files to tracks and
// naming. The rules are pure functions; the service does the I/O and is the
// only writer of artists, albums and tracks.
package music

import (
	"sort"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// DefaultProfileKey is the quality profile an artist gets when none is chosen.
const DefaultProfileKey = "standard"

// DefaultAlbumMinutes is the album length assumed while its tracks are unknown.
const DefaultAlbumMinutes = 45

// ProfileByKey finds a music quality profile by the key of its name.
func ProfileByKey(key string) (decide.QualityProfile, bool) {
	return decide.Lookup(domain.KindMusic, key)
}

// ProfileOrDefault is the artist's profile, or the default one when its key
// no longer names a profile.
func ProfileOrDefault(key string) decide.QualityProfile {
	return movies.OrDefault(domain.KindMusic, key, DefaultProfileKey)
}

// IsReleased reports whether an album has come out by now; an album without
// a date is old enough to have none.
func IsReleased(a domain.Album, now time.Time) bool {
	return a.ReleaseDate == nil || !a.ReleaseDate.After(now)
}

// InProfile reports whether the artist's metadata profile covers an album.
func InProfile(p domain.MetadataProfile, a domain.Album) bool {
	return p.Allows(a.PrimaryType, a.SecondaryTypes)
}

// AlbumOf turns a provider release group into a library album (unmonitored).
func AlbumOf(r plugin.AlbumRecord) domain.Album {
	sec := make([]string, 0, len(r.SecondaryTypes))
	for _, s := range r.SecondaryTypes {
		sec = append(sec, strings.ToLower(s))
	}
	return domain.Album{MBID: r.MBID, Title: r.Title, PrimaryType: domain.ParseAlbumType(r.PrimaryType), SecondaryTypes: sec,
		ReleaseDate: r.FirstRelease}
}

// ApplyMonitor sets the monitored flag of every album for the add-time
// monitoring choice. Only albums inside the metadata profile are ever
// monitored automatically; the others are stored and shown, never searched.
func ApplyMonitor(mode domain.AlbumMonitor, p domain.MetadataProfile, albums []domain.Album, now time.Time) []domain.Album {
	out := make([]domain.Album, len(albums))
	first, latest := bounds(p, albums, now)
	for i, a := range albums {
		a.Monitored = InProfile(p, a) && monitoredBy(mode, a, first, latest, now)
		out[i] = a
	}
	return out
}

func monitoredBy(mode domain.AlbumMonitor, a domain.Album, first, latest string, now time.Time) bool {
	switch mode {
	case domain.AlbumMonitorAll, domain.AlbumMonitorMissing:
		return true // nothing is on disk when an artist is added
	case domain.AlbumMonitorFuture:
		return !IsReleased(a, now)
	case domain.AlbumMonitorFirst:
		return a.MBID == first
	case domain.AlbumMonitorLatest:
		return a.MBID == latest
	}
	return false // existing and none: nothing to watch yet
}

// bounds are the earliest and latest released in-profile albums.
func bounds(p domain.MetadataProfile, albums []domain.Album, now time.Time) (first, latest string) {
	var dated []domain.Album
	for _, a := range albums {
		if InProfile(p, a) && a.ReleaseDate != nil && IsReleased(a, now) {
			dated = append(dated, a)
		}
	}
	if len(dated) == 0 {
		return "", ""
	}
	sort.SliceStable(dated, func(i, j int) bool { return dated[i].ReleaseDate.Before(*dated[j].ReleaseDate) })
	return dated[0].MBID, dated[len(dated)-1].MBID
}

// Merge is the result of refreshing an artist from its provider record.
type Merge struct {
	Albums []domain.Album // every album to write
	Added  int
}

// MergeAlbums reconciles stored albums with the provider's release groups:
// known ones take the new metadata and keep their monitoring and pin; new
// ones are monitored per the artist's monitor-new-items mode, and only when
// the metadata profile covers them. Stored albums gone upstream stay.
func MergeAlbums(a domain.Artist, p domain.MetadataProfile, stored []domain.Album, recs []plugin.AlbumRecord) Merge {
	have := map[string]domain.Album{}
	for _, s := range stored {
		have[s.MBID] = s
	}
	var m Merge
	for _, r := range recs {
		next := AlbumOf(r)
		if old, ok := have[r.MBID]; ok {
			next.ID, next.ArtistID, next.Monitored, next.PinReason = old.ID, old.ArtistID, old.Monitored, old.PinReason
		} else {
			m.Added++
			next.ArtistID, next.Monitored = a.ID, InProfile(p, next) && newMonitored(a, next)
		}
		m.Albums = append(m.Albums, next)
	}
	return m
}

func newMonitored(a domain.Artist, al domain.Album) bool {
	switch a.MonitorNew {
	case domain.MonitorNewItemsAll:
		return true
	case domain.MonitorNewItemsNew:
		return al.ReleaseDate != nil && al.ReleaseDate.After(a.AddedAt)
	}
	return false
}

// preferredFormats rank edition media: digital first, then CD.
var preferredFormats = map[string]int{"digital media": 2, "cd": 1}

// ChooseEdition picks an album's edition: official first, then the
// preferred medium format, then the most common track count among the
// editions, then the earliest date. Editions are judged on their audio
// only, and an edition left without audio is chosen only when every one is.
// ok=false when there is none.
func ChooseEdition(rels []plugin.ReleaseRecord) (plugin.ReleaseRecord, bool) {
	rels = audioEditions(rels)
	if len(rels) == 0 {
		return plugin.ReleaseRecord{}, false
	}
	common := map[int]int{}
	for _, r := range rels {
		common[r.TrackCount()]++
	}
	best := rels[0]
	for _, r := range rels[1:] {
		if editionBefore(r, best, common) {
			best = r
		}
	}
	return best, true
}

func editionBefore(a, b plugin.ReleaseRecord, common map[int]int) bool {
	ka := []int{official(a), formatRank(a), common[a.TrackCount()]}
	kb := []int{official(b), formatRank(b), common[b.TrackCount()]}
	for i := range ka {
		if ka[i] != kb[i] {
			return ka[i] > kb[i]
		}
	}
	return dateOf(a).Before(dateOf(b))
}

func official(r plugin.ReleaseRecord) int {
	if r.Status == "official" {
		return 1
	}
	return 0
}

func formatRank(r plugin.ReleaseRecord) int {
	best := 0
	for _, m := range r.Media {
		best = max(best, preferredFormats[strings.ToLower(m.Format)])
	}
	return best
}

var noDate = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

func dateOf(r plugin.ReleaseRecord) time.Time {
	if r.Date == nil {
		return noDate
	}
	return *r.Date
}

// Edition applies a chosen edition to an album and returns its tracks.
func Edition(a domain.Album, r plugin.ReleaseRecord) (domain.Album, []domain.Track) {
	r = AudioOnly(r)
	a.EditionMBID, a.MediumCount, a.TrackCount, a.DurationSec = r.MBID, len(r.Media), r.TrackCount(), 0
	var tracks []domain.Track
	for _, m := range r.Media {
		for _, t := range m.Tracks {
			tracks = append(tracks, domain.Track{AlbumID: a.ID, RecordingID: t.RecordingMBID, Medium: m.Position,
				MediumFormat: m.Format, Number: t.Position, Title: t.Title, DurationSec: t.LengthSec})
			a.DurationSec += t.LengthSec
		}
	}
	return a, tracks
}

// Minutes is the album length used for size limits.
func Minutes(a domain.Album) int {
	if a.DurationSec <= 0 {
		return DefaultAlbumMinutes
	}
	return max(1, (a.DurationSec+59)/60)
}
