package music

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/naming"
)

// ArtistFolder renders the artist folder from the default template.
func ArtistFolder(a domain.Artist) string {
	name, _ := naming.Preview(naming.DefaultArtistFolder, naming.Values{ArtistName: a.Name})
	return name
}

// AlbumFolder renders the album folder ("Album (Year)") inside the artist's.
func AlbumFolder(al domain.Album) string {
	name, _ := naming.Preview(naming.DefaultAlbumFolder, naming.Values{AlbumTitle: al.Title, ReleaseYear: al.Year()})
	return name
}

// MediumFolder is the per-disc folder ("CD 02"), used only when the album's
// edition has more than one medium.
func MediumFolder(al domain.Album, t domain.Track) string {
	if al.MediumCount <= 1 {
		return ""
	}
	format := t.MediumFormat
	if format == "" || strings.EqualFold(format, "digital media") {
		format = "Disc"
	}
	name, _ := naming.Preview(naming.DefaultMediumFolder, naming.Values{MediumFormat: format, Medium: pad(t.Medium)})
	return name
}

func pad(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// TrackPath is where a track file goes: artist folder, album folder, medium
// folder (multi-disc only) and "Artist - Album - 01 - Title.ext".
func TrackPath(ar domain.Artist, al domain.Album, t domain.Track, ext string) string {
	name, _ := naming.Options{Extension: ext}.Preview(naming.DefaultTrackFile, naming.Values{ArtistName: ar.Name,
		AlbumTitle: al.Title, Track: pad(t.Number), TrackTitle: t.Title})
	return filepath.Join(ar.Path, AlbumFolder(al), MediumFolder(al, t), name)
}

// AlbumPath is the album's folder in the library.
func AlbumPath(ar domain.Artist, al domain.Album) string {
	return filepath.Join(ar.Path, AlbumFolder(al))
}
