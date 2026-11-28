package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// tvDoc is a series as /tv/{id} (and, partly, /search/tv) answers it.
type tvDoc struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
	FirstAirDate string `json:"first_air_date"`
	Overview     string `json:"overview"`
	Status       string `json:"status"`
	PosterPath   string `json:"poster_path"`
	BackdropPath string `json:"backdrop_path"`
	RunTimes     []int  `json:"episode_run_time"`
	Seasons      []struct {
		Number int `json:"season_number"`
	} `json:"seasons"`
	NumberOfSeasons int `json:"number_of_seasons"`
	Networks        []struct {
		Name string `json:"name"`
	} `json:"networks"`
	Genres []struct {
		Name string `json:"name"`
	} `json:"genres"`
	ExternalIDs struct {
		TVDBID int    `json:"tvdb_id"`
		IMDBID string `json:"imdb_id"`
	} `json:"external_ids"`
	AlternativeTitles struct {
		Results []struct {
			Title string `json:"title"`
		} `json:"results"`
	} `json:"alternative_titles"`
}

type tvSearchDoc struct {
	Results []tvDoc `json:"results"`
}

type seasonDoc struct {
	Episodes []struct {
		Season  int    `json:"season_number"`
		Number  int    `json:"episode_number"`
		Name    string `json:"name"`
		AirDate string `json:"air_date"`
		Runtime int    `json:"runtime"`
	} `json:"episodes"`
}

type tvFindDoc struct {
	TVResults []tvDoc `json:"tv_results"`
}

// SearchSeries searches TV series by title.
func (c *Client) SearchSeries(ctx context.Context, query string) ([]plugin.SeriesRecord, error) {
	var doc tvSearchDoc
	if err := c.getJSON(ctx, "/search/tv", url.Values{"query": {query}, "include_adult": {"false"}}, &doc); err != nil {
		return nil, err
	}
	images, err := c.imageBase(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]plugin.SeriesRecord, len(doc.Results))
	for i, d := range doc.Results {
		out[i] = d.record(images)
	}
	return out, nil
}

// Series returns a series with every episode of every season.
func (c *Client) Series(ctx context.Context, ids map[string]string) (plugin.SeriesRecord, error) {
	id, err := c.resolveTV(ctx, ids)
	if err != nil {
		return plugin.SeriesRecord{}, err
	}
	var doc tvDoc
	q := url.Values{"append_to_response": {"external_ids,alternative_titles"}}
	if err := c.getJSON(ctx, "/tv/"+url.PathEscape(id), q, &doc); err != nil {
		return plugin.SeriesRecord{}, err
	}
	images, err := c.imageBase(ctx)
	if err != nil {
		return plugin.SeriesRecord{}, err
	}
	rec := doc.record(images)
	for _, s := range doc.Seasons {
		eps, err := c.season(ctx, id, s.Number, rec.RuntimeMin)
		if err != nil {
			return plugin.SeriesRecord{}, err
		}
		rec.Episodes = append(rec.Episodes, eps...)
	}
	return rec, nil
}

func (c *Client) season(ctx context.Context, id string, n, runtime int) ([]plugin.EpisodeRecord, error) {
	var doc seasonDoc
	if err := c.getJSON(ctx, "/tv/"+url.PathEscape(id)+"/season/"+strconv.Itoa(n), nil, &doc); err != nil {
		return nil, err
	}
	out := make([]plugin.EpisodeRecord, 0, len(doc.Episodes))
	for _, e := range doc.Episodes {
		r := plugin.EpisodeRecord{Season: e.Season, Number: e.Number, Title: e.Name, AirAt: airDate(e.AirDate), RuntimeMin: e.Runtime}
		if r.RuntimeMin == 0 {
			r.RuntimeMin = runtime
		}
		out = append(out, r)
	}
	return out, nil
}

// airDate reads TMDB's date-only air date as midnight UTC.
func airDate(s string) *time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return nil
	}
	return &t
}

var findSources = []struct{ id, source string }{{"tvdb", "tvdb_id"}, {"imdb", "imdb_id"}}

func (c *Client) resolveTV(ctx context.Context, ids map[string]string) (string, error) {
	if id := ids["tmdb"]; id != "" {
		return id, nil
	}
	for _, s := range findSources {
		if ext := ids[s.id]; ext != "" {
			return c.findTV(ctx, ext, s.source)
		}
	}
	return "", fmt.Errorf("series lookup needs a tmdb, tvdb or imdb id: %w", plugin.ErrNotFound)
}

func (c *Client) findTV(ctx context.Context, ext, source string) (string, error) {
	var doc tvFindDoc
	if err := c.getJSON(ctx, "/find/"+url.PathEscape(ext), url.Values{"external_source": {source}}, &doc); err != nil {
		return "", err
	}
	if len(doc.TVResults) == 0 {
		return "", fmt.Errorf("no series with %s %s: %w", source, ext, plugin.ErrNotFound)
	}
	return strconv.Itoa(doc.TVResults[0].ID), nil
}

func (d tvDoc) record(images string) plugin.SeriesRecord {
	r := plugin.SeriesRecord{IDs: map[string]string{"tmdb": strconv.Itoa(d.ID)}, Title: d.Name, Year: yearOf(d.FirstAirDate),
		Status: seriesStatus(d.Status), Overview: d.Overview, PosterURL: posterURL(images, d.PosterPath),
		FanartURL: imageURL(images, backdropSize, d.BackdropPath), SeasonCount: d.NumberOfSeasons, AltTitles: d.altTitles()}
	if d.ExternalIDs.TVDBID > 0 {
		r.IDs["tvdb"] = strconv.Itoa(d.ExternalIDs.TVDBID)
	}
	if d.ExternalIDs.IMDBID != "" {
		r.IDs["imdb"] = d.ExternalIDs.IMDBID
	}
	if len(d.RunTimes) > 0 {
		r.RuntimeMin = d.RunTimes[0]
	}
	if len(d.Networks) > 0 {
		r.Network = d.Networks[0].Name
	}
	for _, g := range d.Genres {
		r.Genres = append(r.Genres, g.Name)
	}
	return r
}

func (d tvDoc) altTitles() []string {
	seen := map[string]bool{d.Name: true, "": true}
	var out []string
	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	add(d.OriginalName)
	for _, t := range d.AlternativeTitles.Results {
		add(t.Title)
	}
	return out
}

// seriesStatus maps TMDB's production status onto continuing/ended/upcoming.
func seriesStatus(s string) string {
	switch s {
	case "Ended", "Canceled":
		return "ended"
	case "Returning Series":
		return "continuing"
	case "Planned", "In Production", "Pilot":
		return "upcoming"
	}
	return ""
}
