package music

import (
	"path/filepath"
	"strings"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/parse"
)

// IndexKey is the normalised artist + album key the wanted index files an
// album under.
func IndexKey(artist, album string) string {
	return movies.Normalize(artist) + "|" + movies.Normalize(album)
}

// Matches reports whether a parsed music release names this album of this
// artist: both names match after normalising and, when the release states a
// year, it is within one of the album's (reissues share titles).
func Matches(ar domain.Artist, al domain.Album, m parse.MusicRelease) bool {
	if movies.Normalize(m.Artist) != movies.Normalize(ar.Name) || movies.Normalize(m.Album) != movies.Normalize(al.Title) {
		return false
	}
	return !m.Year.Known || al.Year() == 0 || abs(m.Year.Value-al.Year()) <= 1
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// AlbumRequest searches one album by artist, album and year; the text form
// ("Artist Album") serves indexers without music search.
func AlbumRequest(ar domain.Artist, al domain.Album, origin domain.SearchOrigin) domain.SearchRequest {
	return domain.SearchRequest{Kind: domain.KindMusic, Mode: domain.SearchMusic, Text: ar.Name + " " + al.Title,
		Titles: []string{al.Title}, Year: al.Year(), IDs: map[string]string{"artist": ar.Name, "album": al.Title, "mbid": al.MBID},
		Origin: origin, ItemRefs: []domain.ItemRef{al.Ref()}}
}

// audioExts are the audio files a music payload is made of.
var audioExts = map[string]bool{".flac": true, ".mp3": true, ".m4a": true, ".alac": true, ".ogg": true, ".oga": true,
	".opus": true, ".wv": true, ".ape": true, ".wav": true, ".aac": true, ".wma": true}

// IsAudio reports whether a file name is an audio file.
func IsAudio(name string) bool { return audioExts[strings.ToLower(filepath.Ext(name))] }

// Target is an album with what the decision needs about it.
type Target struct {
	Artist   domain.Artist
	Album    domain.Album
	Current  *decide.CurrentFile
	Context  decide.Context
	Released bool // the album has come out
}

// Decide runs the shared engine for an album and adds the music rules: the
// release must name this artist and album, a known file list must hold
// audio files and, when the album's tracks are known, about as many of them;
// a discography is never grabbed for one album.
func Decide(t Target, profile decide.QualityProfile, rel domain.Release) domain.Decision {
	item := decide.Item{Ref: t.Album.Ref(), Monitored: t.Artist.Monitored && t.Album.Monitored, Available: t.Released,
		RuntimeMinutes: Minutes(t.Album), Current: t.Current, Profile: profile}
	d := decide.Decide(decide.Input{Release: rel, Parsed: parse.ParseRelease(rel.Title), Item: item, Context: t.Context})
	m := parse.ParseMusic(rel.Title)
	for _, why := range []*domain.Reason{mismatch(t, m), discography(m), noAudio(rel), trackCount(t.Album, rel)} {
		if why != nil {
			d.Reasons = append(d.Reasons, *why)
		}
	}
	d.Accepted, d.Override = len(d.Reasons) == 0, domain.OverrideFor(d.Reasons)
	return d
}

func mismatch(t Target, m parse.MusicRelease) *domain.Reason {
	if Matches(t.Artist, t.Album, m) {
		return nil
	}
	return &domain.Reason{RuleID: "music.mismatch", Category: domain.ReasonPermanent,
		Message: "release does not match " + t.Artist.Name + " - " + t.Album.Title,
		Values:  map[string]any{"artist": m.Artist, "album": m.Album, "year": m.Year.Value}}
}

func discography(m parse.MusicRelease) *domain.Reason {
	if !m.Discography {
		return nil
	}
	return &domain.Reason{RuleID: "music.discography", Category: domain.ReasonPreference,
		Message: "a discography covers more than this album"}
}

func audioFiles(names []string) int {
	n := 0
	for _, f := range names {
		if IsAudio(f) {
			n++
		}
	}
	return n
}

func noAudio(rel domain.Release) *domain.Reason {
	if len(rel.FileList) == 0 || audioFiles(rel.FileList) > 0 {
		return nil
	}
	return &domain.Reason{RuleID: "payload.no_audio", Category: domain.ReasonSafety,
		Message: "the release holds no audio files", Values: map[string]any{"files": rel.FileList}}
}

// trackCount rejects a file list whose audio count is far from the album's
// track count (a single, a deluxe box or another edition entirely).
func trackCount(al domain.Album, rel domain.Release) *domain.Reason {
	n := audioFiles(rel.FileList)
	if n == 0 || al.TrackCount == 0 || (n >= al.TrackCount/2 && n <= al.TrackCount*2) {
		return nil
	}
	return &domain.Reason{RuleID: "music.track_count", Category: domain.ReasonPermanent,
		Message: "the release has a track count far from the album's", Values: map[string]any{"files": n, "tracks": al.TrackCount}}
}
