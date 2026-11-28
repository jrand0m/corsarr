package migrate

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// Pure mappings of a music source onto corsarr's music model.

// musicFloor is the lowest audio quality corsarr's music profiles grab.
const musicFloor decide.QualityID = "mp3-128"

// audioQualityID maps a source audio quality name onto the audio ladder;
// exact=false means it has no rung and the file is recorded as unknown.
func audioQualityID(name string) (decide.QualityID, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, q := range decide.AudioLadder() {
		if strings.ToLower(q.Name) == n {
			return q.ID, true
		}
	}
	switch {
	case n == "unknown":
		return decide.AudioUnknown, true
	case strings.HasPrefix(n, "ogg vorbis"):
		return "vorbis", true
	case strings.HasPrefix(n, "mp3-"):
		if kbps, err := strconv.Atoi(strings.TrimPrefix(n, "mp3-")); err == nil && kbps < 96 {
			return "mp3-low", true
		}
	}
	return decide.AudioUnknown, false
}

// belowMusicFloor lists the allowed source qualities corsarr never grabs.
func belowMusicFloor(allowed []string) []string {
	floor, _ := decide.LookupQuality(musicFloor)
	var out []string
	for _, name := range allowed {
		id, _ := audioQualityID(name)
		if q, ok := decide.LookupQuality(id); ok && q.Rank < floor.Rank {
			out = append(out, name)
		}
	}
	return out
}

// MetaMapping is how one source metadata profile lands in corsarr.
type MetaMapping struct {
	Key    string
	Exact  bool
	Reason string
}

// mapMetaProfile picks the shipped metadata profile allowing the same album
// types, else the closest one by primary types.
func mapMetaProfile(p SrcMetaProfile) MetaMapping {
	primary, secondary := primaryTypes(p.Primary), withoutStudio(p.Secondary)
	notes := ""
	if len(p.Statuses) > 0 && !slices.Equal(p.Statuses, []string{"official"}) {
		notes = fmt.Sprintf("; corsarr does not filter by release status (the source allowed %s)", strings.Join(p.Statuses, ", "))
	}
	for _, m := range domain.MetadataProfiles {
		if slices.Equal(primary, sortedTypes(m.Primary)) && slices.Equal(secondary, sorted(m.Secondary)) {
			return MetaMapping{Key: m.Key, Exact: notes == "", Reason: "same album types as the built-in metadata profile " + m.Key + notes}
		}
	}
	key := closestMeta(primary)
	return MetaMapping{Key: key, Reason: fmt.Sprintf("custom metadata profiles are not supported yet; artists use the built-in %s "+
		"(the source allowed primary types %s and secondary types %s)%s", key, listOrNone(primary), listOrNone(secondary), notes)}
}

func primaryTypes(names []string) []string {
	out := []string{}
	for _, n := range names {
		if t := string(domain.ParseAlbumType(n)); !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return sorted(out)
}

func withoutStudio(names []string) []string {
	out := []string{}
	for _, n := range names {
		if n != studioType {
			out = append(out, n)
		}
	}
	return sorted(out)
}

func sortedTypes(ts []domain.AlbumType) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = string(t)
	}
	return sorted(out)
}

func sorted(list []string) []string {
	out := append([]string{}, list...)
	sort.Strings(out)
	return out
}

func listOrNone(list []string) string {
	if len(list) == 0 {
		return "none"
	}
	return strings.Join(list, ", ")
}

func closestMeta(primary []string) string {
	switch {
	case len(primary) == 0:
		return "none"
	case slices.Contains(primary, string(domain.AlbumTypeEP)) || slices.Contains(primary, string(domain.AlbumTypeSingle)):
		return "standardsingles"
	}
	return domain.DefaultMetadataProfile
}

// metaProfileKey maps an artist's metadata profile id, defaulting when unknown.
func metaProfileKey(profiles map[int]SrcMetaProfile, id int) string {
	if p, ok := profiles[id]; ok {
		return mapMetaProfile(p).Key
	}
	return domain.DefaultMetadataProfile
}

// artistTarget maps a source artist onto corsarr's artist and albums, each
// album with the tracks of its selected edition. Notes say what was folded.
func artistTarget(a SrcArtist, snap Snapshot, rw Rewrite) (domain.Artist, []store.AdoptAlbum, []string) {
	var notes []string
	mn, err := domain.ParseMonitorNewItems(a.MonitorNew)
	if err != nil {
		mn = domain.MonitorNewItemsAll
		notes = append(notes, fmt.Sprintf("monitoring of new albums %q became all", a.MonitorNew))
	}
	sortName := a.SortName
	if sortName == "" {
		sortName = sortTitle(a.Name)
	}
	ar := domain.Artist{MBID: a.MBID, Name: a.Name, SortName: sortName, Disambiguation: a.Disambiguation, Type: a.Type,
		RootFolder: rw.Apply(a.Root), Path: rw.Apply(a.Path), ProfileKey: profileKey(snap, a.ProfileID, domain.KindMusic),
		MetadataProfile: metaProfileKey(snap.MetaProfiles, a.MetaProfileID), MonitorNew: mn, Monitored: a.Monitored,
		Tags: tagLabels(snap.Tags, a.Tags)}
	meta, _ := domain.MetadataProfileByKey(ar.MetadataProfile)
	albums, pinned := albumsTarget(a.Albums, meta)
	if pinned > 0 {
		notes = append(notes, fmt.Sprintf("%d monitored albums outside the metadata profile stay monitored", pinned))
	}
	if skipped := len(a.Albums) - len(albums); skipped > 0 {
		notes = append(notes, fmt.Sprintf("%d albums without a MusicBrainz id were left out", skipped))
	}
	return ar, albums, notes
}

// albumsTarget maps the albums with a MusicBrainz id; a monitored album the
// metadata profile does not cover is pinned so the profile leaves it alone.
func albumsTarget(list []SrcAlbum, meta domain.MetadataProfile) ([]store.AdoptAlbum, int) {
	out := make([]store.AdoptAlbum, 0, len(list))
	pinned := 0
	for _, s := range list {
		if s.MBID == "" {
			continue
		}
		al := domain.Album{MBID: s.MBID, Title: s.Title, PrimaryType: domain.ParseAlbumType(s.Type), SecondaryTypes: s.SecondaryTypes,
			ReleaseDate: s.ReleaseDate, Monitored: s.Monitored}
		if al.Monitored && !meta.Allows(al.PrimaryType, al.SecondaryTypes) {
			al.PinReason = domain.PinImport
			pinned++
		}
		out = append(out, editionOf(al, s))
	}
	return out, pinned
}

// editionOf seeds an album's selected edition and its tracks from the source.
func editionOf(al domain.Album, s SrcAlbum) store.AdoptAlbum {
	if s.EditionMBID == "" || len(s.Tracks) == 0 {
		return store.AdoptAlbum{Album: al}
	}
	al.EditionMBID, al.MediumCount, al.TrackCount = s.EditionMBID, s.MediumCount, len(s.Tracks)
	tracks := make([]domain.Track, len(s.Tracks))
	for i, t := range s.Tracks {
		tracks[i] = domain.Track{RecordingID: t.RecordingID, Medium: t.Medium, Number: t.Number, Title: t.Title, DurationSec: t.DurationSec}
		al.DurationSec += t.DurationSec
		al.MediumCount = max(al.MediumCount, t.Medium)
	}
	return store.AdoptAlbum{Album: al, Tracks: tracks}
}

// ArtistTargetHash fingerprints the settings and album monitoring a
// migration writes for an artist; it tells a corsarr edit from an untouched
// object.
func ArtistTargetHash(a domain.Artist, albums []domain.Album) string {
	mon := make([]string, 0, len(albums))
	for _, al := range albums {
		mon = append(mon, fmt.Sprintf("%s=%t", al.MBID, al.Monitored))
	}
	sort.Strings(mon)
	return hashOf([]any{a.Monitored, a.ProfileKey, a.MetadataProfile, a.MonitorNew, strings.Join(a.Tags, ","), a.RootFolder,
		a.Path, mon})
}

func albumsOf(list []store.AdoptAlbum) []domain.Album {
	out := make([]domain.Album, len(list))
	for i, a := range list {
		out[i] = a.Album
	}
	return out
}
