package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
)

type fake struct {
	srv     *httptest.Server
	mu      sync.Mutex
	reqs    []*http.Request
	limited int // answer this many requests with 429 first
}

func newFake(t *testing.T) *fake {
	t.Helper()
	f := &fake{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func read(name string) []byte {
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		panic(err)
	}
	return b
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.reqs = append(f.reqs, r)
	limited := f.limited > 0
	if limited {
		f.limited--
	}
	f.mu.Unlock()
	if limited {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	if r.URL.Query().Get("api_key") != "key" && r.Header.Get("Authorization") != "Bearer eyJtoken" {
		http.Error(w, `{"status_code":7,"status_message":"Invalid API key"}`, http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/configuration":
		_, _ = w.Write(read("configuration.json"))
	case "/search/movie":
		_, _ = w.Write(read("search.json"))
	case "/movie/603":
		_, _ = w.Write(read("movie.json"))
	case "/find/tt0133093":
		_, _ = w.Write([]byte(`{"movie_results":[{"id":603}]}`))
	case "/find/tt0000000":
		_, _ = w.Write([]byte(`{"movie_results":[]}`))
	default:
		http.NotFound(w, r)
	}
}

func (f *fake) last() *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reqs[len(f.reqs)-1]
}

func newClient(t *testing.T, base string, extra plugin.Settings) *Client {
	t.Helper()
	s := plugin.Settings{"apiKey": "key", "baseUrl": base}
	for k, v := range extra {
		s[k] = v
	}
	p, err := New(plugin.Instance{Key: "tmdb", Settings: s, Env: conformance.Env(http.DefaultClient)})
	if err != nil {
		t.Fatal(err)
	}
	return p.(*Client)
}

func TestConformance(t *testing.T) {
	reg, ok := plugin.Default.Get(ID)
	if !ok {
		t.Fatal("tmdb not registered")
	}
	conformance.Manifest(t, reg.Manifest, plugin.KindMetadata)
	f := newFake(t)
	conformance.Metadata(t, newClient(t, f.srv.URL, nil), "matrix", 1999)
}

func TestErrorMapping(t *testing.T) {
	conformance.ErrorMapping(t, func(url string) plugin.Tester { return newClient(t, url, nil) },
		conformance.ErrorCase{Name: "garbage", Want: plugin.ClassParse,
			Handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }},
		conformance.ErrorCase{Name: "other 4xx", Want: plugin.ClassServerError,
			Handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }},
	)
}

func TestSearchMapping(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f.srv.URL, plugin.Settings{"region": "gb", "language": "de-DE"})
	res, err := c.SearchMovies(t.Context(), "the matrix", 1999)
	if err != nil || len(res) != 2 {
		t.Fatalf("%v %d", err, len(res))
	}
	r := res[0]
	if r.IDs["tmdb"] != "603" || r.Title != "The Matrix" || r.Year != 1999 || r.PosterURL != "https://image.tmdb.org/t/p/w500/poster.jpg" || r.Overview == "" {
		t.Errorf("%+v", r)
	}
	if res[1].Year != 0 || res[1].PosterURL != "" {
		t.Errorf("missing date/poster %+v", res[1])
	}
	var search *http.Request
	for _, q := range f.reqs {
		if q.URL.Path == "/search/movie" {
			search = q
		}
	}
	v := search.URL.Query()
	if v.Get("query") != "the matrix" || v.Get("year") != "1999" || v.Get("region") != "GB" || v.Get("language") != "de-DE" || v.Get("include_adult") != "false" {
		t.Errorf("params %v", v)
	}
	if _, err := c.SearchMovies(t.Context(), "x", 0); err != nil || f.last().URL.Query().Get("year") != "" {
		t.Error("no year param when year is 0")
	}
}

func TestMovieFullRecordUSRegion(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f.srv.URL, plugin.Settings{"region": "us"})
	r, err := c.Movie(t.Context(), map[string]string{"tmdb": "603"})
	if err != nil {
		t.Fatal(err)
	}
	if r.IDs["imdb"] != "tt0133093" || r.Year != 1999 || r.RuntimeMin != 136 || r.Status != "Released" || len(r.Genres) != 2 ||
		r.FanartURL != "https://image.tmdb.org/t/p/w1280/backdrop.jpg" {
		t.Errorf("%+v", r)
	}
	if len(r.AltTitles) != 1 || r.AltTitles[0] != "Matrix" {
		t.Errorf("alt titles deduped %v", r.AltTitles)
	}
	day := func(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }
	if r.InCinemas == nil || !r.InCinemas.Equal(day("1999-03-31")) || r.Digital == nil || !r.Digital.Equal(day("2008-12-01")) ||
		r.Physical == nil || !r.Physical.Equal(day("1999-09-21")) {
		t.Errorf("dates %v %v %v", r.InCinemas, r.Digital, r.Physical)
	}
	q := f.reqs[len(f.reqs)-2].URL.Query() // movie request precedes the configuration lookup
	if q.Get("append_to_response") != "release_dates,external_ids,alternative_titles" {
		t.Errorf("append_to_response %q", q.Get("append_to_response"))
	}
}

func TestMovieEarliestWorldwideWithoutRegion(t *testing.T) {
	f := newFake(t)
	r, err := newClient(t, f.srv.URL, nil).Movie(t.Context(), map[string]string{"tmdb": "603"})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.InCinemas.Format("2006-01-02"); got != "1999-03-31" {
		t.Errorf("earliest theatrical %s", got)
	}
	if got := r.Digital.Format("2006-01-02"); got != "2007-01-01" {
		t.Errorf("earliest digital %s", got)
	}
}

func TestMovieRegionWithoutReleases(t *testing.T) {
	f := newFake(t)
	r, err := newClient(t, f.srv.URL, plugin.Settings{"region": "FR"}).Movie(t.Context(), map[string]string{"tmdb": "603"})
	if err != nil || r.InCinemas != nil || r.Digital != nil || r.Physical != nil {
		t.Errorf("%+v %v", r, err)
	}
}

func TestMovieByIMDBID(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f.srv.URL, nil)
	r, err := c.Movie(t.Context(), map[string]string{"imdb": "tt0133093"})
	if err != nil || r.Title != "The Matrix" {
		t.Fatalf("%+v %v", r, err)
	}
	for _, ids := range []map[string]string{{"imdb": "tt0000000"}, {}, {"tmdb": "999"}} {
		if _, err := c.Movie(t.Context(), ids); !errors.Is(err, plugin.ErrNotFound) || plugin.ClassOf(err) != "" {
			t.Errorf("%v: not-found must be plain ErrNotFound, got %v", ids, err)
		}
	}
}

func TestBearerTokenKeepsKeyOutOfURL(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f.srv.URL, plugin.Settings{"apiKey": "eyJtoken"})
	if err := c.Test(t.Context()); err != nil {
		t.Fatal(err)
	}
	r := f.last()
	if r.Header.Get("Authorization") != "Bearer eyJtoken" || r.URL.Query().Get("api_key") != "" {
		t.Errorf("auth %v %v", r.Header, r.URL.RawQuery)
	}
}

func TestRateLimitRetriesOnce(t *testing.T) {
	f := newFake(t)
	f.limited = 1
	if err := newClient(t, f.srv.URL, nil).Test(t.Context()); err != nil {
		t.Fatalf("one 429 must be retried: %v", err)
	}
	f.limited = 5
	if err := newClient(t, f.srv.URL, nil).Test(t.Context()); plugin.ClassOf(err) != plugin.ClassRateLimited {
		t.Fatalf("persistent 429: %v", err)
	}
}

func TestRetryAfterParsing(t *testing.T) {
	h := http.Header{}
	if retryAfter(h) != time.Second {
		t.Error("default")
	}
	h.Set("Retry-After", "3")
	if retryAfter(h) != 3*time.Second {
		t.Error("seconds")
	}
	h.Set("Retry-After", "9999")
	if retryAfter(h) != maxRetryAfter {
		t.Error("capped")
	}
	h.Set("Retry-After", "-4")
	if retryAfter(h) != time.Second {
		t.Error("negative")
	}
}

func TestPacingSpacesRequests(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f.srv.URL, nil)
	start := time.Now()
	for range 3 {
		if _, err := c.do(t.Context(), "/configuration", nil); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 2*minInterval {
		t.Errorf("three requests took %v, want at least %v", d, 2*minInterval)
	}
}

func TestPacingHonoursContext(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f.srv.URL, nil)
	c.last = time.Now().Add(time.Hour)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := c.Test(ctx); plugin.ClassOf(err) != plugin.ClassTimeout {
		t.Errorf("%v", err)
	}
	if err := sleep(t.Context(), 0); err != nil {
		t.Error(err)
	}
}

func TestSettingsValidation(t *testing.T) {
	for _, s := range []plugin.Settings{{}, {"apiKey": "k", "baseUrl": "ftp://x"}, {"apiKey": "k", "baseUrl": "::"}} {
		if _, err := New(plugin.Instance{Settings: s}); plugin.ClassOf(err) != plugin.ClassConfig {
			t.Errorf("%v: %v", s, err)
		}
	}
	p, err := New(plugin.Instance{Settings: plugin.Settings{"apiKey": "k"}})
	if err != nil {
		t.Fatal(err)
	}
	c := p.(*Client)
	if c.base != defaultBase || c.language != "en-US" || c.region != "" {
		t.Errorf("defaults %+v", c)
	}
}

func TestImageBaseFallbackAndCache(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"images":{}}`))
	}))
	defer srv.Close()
	c := newClient(t, srv.URL, nil)
	for range 2 {
		if b, err := c.imageBase(t.Context()); err != nil || b != defaultImages {
			t.Fatalf("%q %v", b, err)
		}
	}
	if hits != 1 {
		t.Errorf("configuration fetched %d times", hits)
	}
}

func TestHelpers(t *testing.T) {
	if yearOf("") != 0 || yearOf("19") != 0 || yearOf("2001-02-03") != 2001 {
		t.Error("yearOf")
	}
	if posterURL("https://x/t/p/", "") != "" || !strings.HasSuffix(posterURL("https://x/t/p", "/a.jpg"), "/w500/a.jpg") {
		t.Error("posterURL")
	}
	u, _ := url.Parse("http://h/x?api_key=SECRET")
	if strings.Contains(plugin.Redact(u), "SECRET") {
		t.Error("redact")
	}
}
