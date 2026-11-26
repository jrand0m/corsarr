package migrate

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// studioType is the secondary type a music source gives albums that have
// none; corsarr represents it as an empty list.
const studioType = "studio"

func readMetaProfiles(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	var list []wMetadataProfile
	if err := c.get(ctx, "/metadataprofile", nil, &list); err != nil {
		return err
	}
	for _, p := range list {
		s.MetaProfiles[p.ID] = SrcMetaProfile{ID: p.ID, Name: p.Name, Primary: allowedTypes(p.PrimaryAlbumTypes),
			Secondary: allowedTypes(p.SecondaryAlbumTypes), Statuses: allowedTypes(p.ReleaseStatuses)}
	}
	return nil
}

// allowedTypes lists the allowed names of a metadata profile list, lower case.
func allowedTypes(list []wAllowedType) []string {
	out := []string{}
	for _, t := range list {
		name := ""
		switch {
		case t.AlbumType != nil:
			name = t.AlbumType.Name
		case t.ReleaseStatus != nil:
			name = t.ReleaseStatus.Name
		}
		if t.Allowed && name != "" {
			out = append(out, strings.ToLower(name))
		}
	}
	return out
}

func readArtists(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	var list []wArtist
	if err := c.get(ctx, "/artist", nil, &list); err != nil {
		return err
	}
	for _, w := range list {
		a, err := artistOf(ctx, c, w)
		if err != nil {
			return err
		}
		s.Artists = append(s.Artists, a)
	}
	return nil
}

// artistOf reads one artist's albums, tracks and track files and parses them.
func artistOf(ctx context.Context, c *Client, w wArtist) (SrcArtist, error) {
	q := url.Values{"artistId": {strconv.Itoa(w.ID)}}
	var albums []wAlbum
	var tracks []wTrack
	var files []wTrackFile
	for _, r := range []struct {
		path string
		out  any
	}{{"/album", &albums}, {"/track", &tracks}, {"/trackfile", &files}} {
		if err := c.get(ctx, r.path, q, r.out); err != nil {
			return SrcArtist{}, err
		}
	}
	a := SrcArtist{ID: w.ID, MBID: w.ForeignArtistID, Name: w.ArtistName, SortName: w.SortName, Disambiguation: w.Disambiguation,
		Type: w.ArtistType, Path: cleanPath(w.Path), Root: cleanPath(w.RootFolderPath), ProfileID: w.QualityProfileID,
		MetaProfileID: w.MetadataProfileID, Monitored: w.Monitored, MonitorNew: w.MonitorNewItems, Tags: w.Tags}
	byAlbum := map[int][]SrcTrack{}
	for _, t := range tracks {
		byAlbum[t.AlbumID] = append(byAlbum[t.AlbumID], trackOf(t))
	}
	for _, al := range albums {
		a.Albums = append(a.Albums, albumOf(al, byAlbum[al.ID]))
	}
	for _, f := range files {
		a.Files = append(a.Files, SrcTrackFile{SrcFile: fileOf(f.wFile), AlbumID: f.AlbumID})
	}
	return a, nil
}

func albumOf(w wAlbum, tracks []SrcTrack) SrcAlbum {
	al := SrcAlbum{ID: w.ID, MBID: w.ForeignAlbumID, Title: w.Title, Type: w.AlbumType, ReleaseDate: w.ReleaseDate,
		Monitored: w.Monitored, SecondaryTypes: []string{}, Tracks: tracks}
	for _, t := range w.SecondaryTypes {
		if n := strings.ToLower(string(t)); n != studioType && n != "" {
			al.SecondaryTypes = append(al.SecondaryTypes, n)
		}
	}
	for _, r := range w.Releases {
		if r.Monitored {
			al.EditionMBID, al.MediumCount = r.ForeignReleaseID, r.MediumCount
		}
	}
	return al
}

// trackOf reads a track's place: its position on the medium, else the
// leading digits of its printed number ("A3" on vinyl is 3).
func trackOf(t wTrack) SrcTrack {
	n := t.AbsoluteTrackNumber
	if n <= 0 {
		n, _ = strconv.Atoi(strings.TrimLeft(t.TrackNumber, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"))
	}
	return SrcTrack{ID: t.ID, RecordingID: t.ForeignRecordingID, Medium: max(t.MediumNumber, 1), Number: n, Title: t.Title,
		DurationSec: (t.Duration + 500) / 1000, FileID: t.TrackFileID}
}
