package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
)

type movieDoc struct {
	ID            int    `json:"id"`
	IMDBID        string `json:"imdb_id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Overview      string `json:"overview"`
	ReleaseDate   string `json:"release_date"`
	Runtime       int    `json:"runtime"`
	Status        string `json:"status"`
	PosterPath    string `json:"poster_path"`
	Genres        []struct {
		Name string `json:"name"`
	} `json:"genres"`
	ExternalIDs struct {
		IMDBID string `json:"imdb_id"`
	} `json:"external_ids"`
	AlternativeTitles struct {
		Titles []struct {
			Title string `json:"title"`
		} `json:"titles"`
	} `json:"alternative_titles"`
	ReleaseDates struct {
		Results []countryReleases `json:"results"`
	} `json:"release_dates"`
}

type countryReleases struct {
	Country string `json:"iso_3166_1"`
	Dates   []struct {
		Type int    `json:"type"`
		Date string `json:"release_date"`
	} `json:"release_dates"`
}

type searchDoc struct {
	Results []movieDoc `json:"results"`
}

type findDoc struct {
	MovieResults []movieDoc `json:"movie_results"`
}

// SearchMovies searches by title, optionally narrowed to a release year.
func (c *Client) SearchMovies(ctx context.Context, query string, year int) ([]plugin.MovieRecord, error) {
	q := url.Values{"query": {query}, "include_adult": {"false"}}
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	if c.region != "" {
		q.Set("region", c.region)
	}
	var doc searchDoc
	if err := c.getJSON(ctx, "/search/movie", q, &doc); err != nil {
		return nil, err
	}
	images, err := c.imageBase(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]plugin.MovieRecord, len(doc.Results))
	for i, d := range doc.Results {
		out[i] = d.record(images, c.region)
	}
	return out, nil
}

// Movie returns the full record for ids["tmdb"], or resolves ids["imdb"] first.
func (c *Client) Movie(ctx context.Context, ids map[string]string) (plugin.MovieRecord, error) {
	id, err := c.resolveID(ctx, ids)
	if err != nil {
		return plugin.MovieRecord{}, err
	}
	var doc movieDoc
	q := url.Values{"append_to_response": {"release_dates,external_ids,alternative_titles"}}
	if err := c.getJSON(ctx, "/movie/"+url.PathEscape(id), q, &doc); err != nil {
		return plugin.MovieRecord{}, err
	}
	images, err := c.imageBase(ctx)
	if err != nil {
		return plugin.MovieRecord{}, err
	}
	return doc.record(images, c.region), nil
}

func (c *Client) resolveID(ctx context.Context, ids map[string]string) (string, error) {
	if id := ids["tmdb"]; id != "" {
		return id, nil
	}
	imdb := ids["imdb"]
	if imdb == "" {
		return "", fmt.Errorf("movie lookup needs a tmdb or imdb id: %w", plugin.ErrNotFound)
	}
	var doc findDoc
	if err := c.getJSON(ctx, "/find/"+url.PathEscape(imdb), url.Values{"external_source": {"imdb_id"}}, &doc); err != nil {
		return "", err
	}
	if len(doc.MovieResults) == 0 {
		return "", fmt.Errorf("no movie with imdb id %s: %w", imdb, plugin.ErrNotFound)
	}
	return strconv.Itoa(doc.MovieResults[0].ID), nil
}

func (d movieDoc) record(images, region string) plugin.MovieRecord {
	r := plugin.MovieRecord{
		IDs:   map[string]string{"tmdb": strconv.Itoa(d.ID)},
		Title: d.Title, OriginalTitle: d.OriginalTitle, Year: yearOf(d.ReleaseDate),
		Overview: d.Overview, RuntimeMin: d.Runtime, Status: d.Status,
		PosterURL: posterURL(images, d.PosterPath), AltTitles: d.altTitles(),
	}
	if imdb := firstNonEmpty(d.ExternalIDs.IMDBID, d.IMDBID); imdb != "" {
		r.IDs["imdb"] = imdb
	}
	for _, g := range d.Genres {
		r.Genres = append(r.Genres, g.Name)
	}
	r.InCinemas, r.Digital, r.Physical = releaseDates(d.ReleaseDates.Results, region)
	return r
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}

func posterURL(images, path string) string {
	if path == "" {
		return ""
	}
	return strings.TrimRight(images, "/") + "/" + posterSize + path
}

func (d movieDoc) altTitles() []string {
	seen := map[string]bool{d.Title: true, "": true}
	var out []string
	for _, t := range d.AlternativeTitles.Titles {
		if !seen[t.Title] {
			seen[t.Title] = true
			out = append(out, t.Title)
		}
	}
	return out
}

// TMDB release types: 1 premiere, 2 theatrical (limited), 3 theatrical, 4 digital, 5 physical, 6 TV.
const (
	typeLimited  = 2
	typeTheater  = 3
	typeDigital  = 4
	typePhysical = 5
)

// releaseDates picks the earliest cinema, digital and physical dates for the
// region (every region when it is empty).
func releaseDates(results []countryReleases, region string) (cinema, digital, physical *time.Time) {
	for _, cr := range results {
		if region != "" && !strings.EqualFold(cr.Country, region) {
			continue
		}
		for _, rd := range cr.Dates {
			t, err := time.Parse(time.RFC3339, rd.Date)
			if err != nil {
				continue
			}
			t = t.UTC()
			switch rd.Type {
			case typeLimited, typeTheater:
				cinema = earlier(cinema, t)
			case typeDigital:
				digital = earlier(digital, t)
			case typePhysical:
				physical = earlier(physical, t)
			}
		}
	}
	return cinema, digital, physical
}

func earlier(cur *time.Time, t time.Time) *time.Time {
	if cur == nil || t.Before(*cur) {
		return &t
	}
	return cur
}
