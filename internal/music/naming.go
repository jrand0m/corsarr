package music

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/naming"
)

// ArtistFolder renders the artist folder from the naming's folder template.
func ArtistFolder(a domain.Artist, sch naming.Scheme) string {
	return sch.FolderName(naming.Values{ArtistName: a.Name}, naming.DefaultArtistFolder)
}

// AlbumFolder renders the album folder ("Album (Year)" by default) inside the artist's.
func AlbumFolder(ar domain.Artist, al domain.Album, sch naming.Scheme) string {
	return sch.SubName(naming.Values{ArtistName: ar.Name, AlbumTitle: al.Title, ReleaseYear: al.Year()}, naming.DefaultAlbumFolder)
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
// folder (multi-disc only) and the track file name ("Artist - Album - 01 -
// Title.ext" by default; the original file's own name when renaming is off).
func TrackPath(ar domain.Artist, al domain.Album, t domain.Track, ext, original string, sch naming.Scheme) string {
	name := sch.FileName(naming.Values{ArtistName: ar.Name, AlbumTitle: al.Title, ReleaseYear: al.Year(), Track: pad(t.Number),
		TrackTitle: t.Title, Medium: pad(t.Medium), MediumFormat: t.MediumFormat}, naming.DefaultTrackFile, ext, original)
	return filepath.Join(ar.Path, AlbumFolder(ar, al, sch), MediumFolder(al, t), name)
}

// AlbumPath is the album's folder in the library.
func AlbumPath(ar domain.Artist, al domain.Album, sch naming.Scheme) string {
	return filepath.Join(ar.Path, AlbumFolder(ar, al, sch))
}
