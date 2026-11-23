package tmdb

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// tvFake serves a two-season series recorded in the shape of the public
// TMDB v3 TV endpoints (data invented for the test).
func tvFake(t *testing.T) *httptest.Server {
	t.Helper()
	answers := map[string]string{
		"/configuration": `{"images":{"secure_base_url":"https://img.example/"}}`,
		"/search/tv":     `{"results":[{"id":77,"name":"Example Show","first_air_date":"2019-04-02","overview":"o","poster_path":"/p.jpg"},{"id":78,"name":"Other"}]}`,
		"/tv/77": `{"id":77,"name":"Example Show","original_name":"Beispiel","first_air_date":"2019-04-02","status":"Returning Series",
			"episode_run_time":[42],"number_of_seasons":2,"seasons":[{"season_number":0},{"season_number":1}],
			"networks":[{"name":"Net"}],"genres":[{"name":"Drama"}],"external_ids":{"tvdb_id":321,"imdb_id":"tt0077"},
			"alternative_titles":{"results":[{"title":"Example Show"},{"title":"Example"}]}}`,
		"/tv/77/season/0": `{"episodes":[{"season_number":0,"episode_number":1,"name":"Special","air_date":""}]}`,
		"/tv/77/season/1": `{"episodes":[{"season_number":1,"episode_number":1,"name":"Pilot","air_date":"2019-04-02","runtime":55},
			{"season_number":1,"episode_number":2,"name":"Two","air_date":"2019-04-09"}]}`,
		"/find/321":       `{"tv_results":[{"id":77}]}`,
		"/find/tt0077":    `{"tv_results":[{"id":77}]}`,
		"/find/tt9999999": `{"tv_results":[]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := answers[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSearchSeries(t *testing.T) {
	c := newClient(t, tvFake(t).URL, nil)
	res, err := c.SearchSeries(t.Context(), "example")
	if err != nil || len(res) != 2 {
		t.Fatalf("%v %d", err, len(res))
	}
	if r := res[0]; r.IDs["tmdb"] != "77" || r.Title != "Example Show" || r.Year != 2019 || r.PosterURL != "https://img.example/w500/p.jpg" {
		t.Fatalf("%+v", r)
	}
}

func TestSeriesFullRecordWithEpisodes(t *testing.T) {
	c := newClient(t, tvFake(t).URL, nil)
	r, err := c.Series(t.Context(), map[string]string{"tmdb": "77"})
	if err != nil {
		t.Fatal(err)
	}
	if r.IDs["tvdb"] != "321" || r.IDs["imdb"] != "tt0077" || r.Status != "continuing" || r.Network != "Net" ||
		r.RuntimeMin != 42 || r.SeasonCount != 2 || len(r.Genres) != 1 || len(r.AltTitles) != 2 {
		t.Fatalf("%+v", r)
	}
	if len(r.Episodes) != 3 {
		t.Fatalf("episodes %+v", r.Episodes)
	}
	special, pilot, two := r.Episodes[0], r.Episodes[1], r.Episodes[2]
	if special.AirAt != nil || special.Season != 0 {
		t.Errorf("special without a date is TBA: %+v", special)
	}
	if pilot.RuntimeMin != 55 || !pilot.AirAt.Equal(time.Date(2019, 4, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("pilot %+v", pilot)
	}
	if two.RuntimeMin != 42 {
		t.Errorf("episode runtime falls back to the series runtime: %+v", two)
	}
}

func TestSeriesByExternalIDs(t *testing.T) {
	c := newClient(t, tvFake(t).URL, nil)
	for _, ids := range []map[string]string{{"tvdb": "321"}, {"imdb": "tt0077"}} {
		if r, err := c.Series(t.Context(), ids); err != nil || r.Title != "Example Show" {
			t.Errorf("%v: %+v %v", ids, r, err)
		}
	}
	for _, ids := range []map[string]string{{"imdb": "tt9999999"}, {}, {"tmdb": "1"}} {
		if _, err := c.Series(t.Context(), ids); !errors.Is(err, plugin.ErrNotFound) {
			t.Errorf("%v: want not found, got %v", ids, err)
		}
	}
}

func TestSeriesStatusMapping(t *testing.T) {
	for in, want := range map[string]string{"Ended": "ended", "Canceled": "ended", "Returning Series": "continuing",
		"In Production": "upcoming", "Planned": "upcoming", "Pilot": "upcoming", "": ""} {
		if got := seriesStatus(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}
