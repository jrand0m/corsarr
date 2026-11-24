package musicbrainz

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
)

type artistDoc struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	SortName       string `json:"sort-name"`
	Disambiguation string `json:"disambiguation"`
	Type           string `json:"type"`
	Country        string `json:"country"`
	LifeSpan       struct {
		Begin string `json:"begin"`
	} `json:"life-span"`
}

type artistSearchDoc struct {
	Artists []artistDoc `json:"artists"`
}

type releaseGroupDoc struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	PrimaryType    string   `json:"primary-type"`
	SecondaryTypes []string `json:"secondary-types"`
	FirstRelease   string   `json:"first-release-date"`
}

type releaseGroupPage struct {
	Count  int               `json:"release-group-count"`
	Groups []releaseGroupDoc `json:"release-groups"`
}

type releaseDoc struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Date    string `json:"date"`
	Country string `json:"country"`
	Media   []struct {
		Position int    `json:"position"`
		Format   string `json:"format"`
		Tracks   []struct {
			Position  int    `json:"position"`
			Title     string `json:"title"`
			Length    int    `json:"length"` // milliseconds
			Recording struct {
				ID     string `json:"id"`
				Length int    `json:"length"`
			} `json:"recording"`
		} `json:"tracks"`
	} `json:"media"`
}

type releasePage struct {
	Releases []releaseDoc `json:"releases"`
}

func (d artistDoc) record() plugin.ArtistRecord {
	year, _ := strconv.Atoi(first4(d.LifeSpan.Begin))
	sort := d.SortName
	if sort == "" {
		sort = d.Name
	}
	return plugin.ArtistRecord{MBID: d.ID, Name: d.Name, SortName: sort, Disambiguation: d.Disambiguation, Type: d.Type,
		Country: d.Country, BeginYear: year}
}

func first4(s string) string {
	if len(s) < 4 {
		return ""
	}
	return s[:4]
}

// Date reads a partial MusicBrainz date ("2020", "2020-05", "2020-05-01")
// as the earliest day it can mean; nil when absent or malformed.
func Date(s string) *time.Time {
	for _, layout := range []string{time.DateOnly, "2006-01", "2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// SearchArtists searches artists by name.
func (c *Client) SearchArtists(ctx context.Context, query string) ([]plugin.ArtistRecord, error) {
	var doc artistSearchDoc
	if err := c.getJSON(ctx, "/artist", url.Values{"query": {query}, "limit": {"25"}}, &doc); err != nil {
		return nil, err
	}
	out := make([]plugin.ArtistRecord, len(doc.Artists))
	for i, a := range doc.Artists {
		out[i] = a.record()
	}
	return out, nil
}

// Artist returns an artist with every release group, paged in hundreds.
func (c *Client) Artist(ctx context.Context, mbid string) (plugin.ArtistRecord, error) {
	var doc artistDoc
	if err := c.getJSON(ctx, "/artist/"+url.PathEscape(mbid), nil, &doc); err != nil {
		return plugin.ArtistRecord{}, err
	}
	rec := doc.record()
	for page := 0; page < maxPages; page++ {
		var p releaseGroupPage
		q := url.Values{"artist": {mbid}, "limit": {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(page * pageSize)}}
		if err := c.getJSON(ctx, "/release-group", q, &p); err != nil {
			return rec, err
		}
		for _, g := range p.Groups {
			rec.Albums = append(rec.Albums, plugin.AlbumRecord{MBID: g.ID, Title: g.Title, PrimaryType: g.PrimaryType,
				SecondaryTypes: g.SecondaryTypes, FirstRelease: Date(g.FirstRelease)})
		}
		if len(p.Groups) < pageSize || len(rec.Albums) >= p.Count {
			break
		}
	}
	return rec, nil
}

// Releases returns up to one hundred editions of a release group with their
// media and tracks.
func (c *Client) Releases(ctx context.Context, releaseGroupMBID string) ([]plugin.ReleaseRecord, error) {
	var p releasePage
	q := url.Values{"release-group": {releaseGroupMBID}, "inc": {"media recordings"}, "limit": {strconv.Itoa(pageSize)}}
	if err := c.getJSON(ctx, "/release", q, &p); err != nil {
		return nil, err
	}
	out := make([]plugin.ReleaseRecord, len(p.Releases))
	for i, r := range p.Releases {
		out[i] = r.record()
	}
	return out, nil
}

func (r releaseDoc) record() plugin.ReleaseRecord {
	rec := plugin.ReleaseRecord{MBID: r.ID, Title: r.Title, Status: strings.ToLower(r.Status), Date: Date(r.Date), Country: r.Country,
		Media: make([]plugin.MediumRecord, len(r.Media))}
	for i, m := range r.Media {
		med := plugin.MediumRecord{Position: m.Position, Format: m.Format, Tracks: make([]plugin.TrackRecord, len(m.Tracks))}
		for j, t := range m.Tracks {
			ms := t.Length
			if ms == 0 {
				ms = t.Recording.Length
			}
			med.Tracks[j] = plugin.TrackRecord{RecordingMBID: t.Recording.ID, Position: t.Position, Title: t.Title, LengthSec: (ms + 500) / 1000}
		}
		rec.Media[i] = med
	}
	return rec
}
