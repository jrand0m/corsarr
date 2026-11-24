package musicbrainz

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
)

const (
	artistMBID = "a0000000-0000-0000-0000-000000000001"
	groupMBID  = "g0000000-0000-0000-0000-000000000001"
)

type fake struct {
	srv   *httptest.Server
	mu    sync.Mutex
	reqs  []*http.Request
	busy  int // answer this many requests with 503 first
	pages int
}

func newFake(t *testing.T) *fake {
	f := &fake{pages: 1}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.reqs = append(f.reqs, r)
	busy := f.busy > 0
	f.busy--
	f.mu.Unlock()
	if busy {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	switch {
	case r.URL.Path == "/artist" && r.URL.Query().Get("query") == "garbage":
		_, _ = w.Write([]byte(`{`))
	case r.URL.Path == "/artist":
		_, _ = w.Write([]byte(`{"artists":[{"id":"` + artistMBID + `","name":"Example Artist","sort-name":"Artist, Example",
			"disambiguation":"the band","type":"Group","country":"GB","life-span":{"begin":"1999-04"}}]}`))
	case r.URL.Path == "/artist/"+artistMBID:
		_, _ = w.Write([]byte(`{"id":"` + artistMBID + `","name":"Example Artist","life-span":{}}`))
	case r.URL.Path == "/release-group":
		f.groups(w, r)
	case r.URL.Path == "/release" && r.URL.Query().Get("release-group") == groupMBID:
		_, _ = w.Write([]byte(`{"releases":[{"id":"r1","title":"First Album","status":"Official","date":"2020-03","country":"XW",
			"media":[{"position":1,"format":"Digital Media","tracks":[
			{"position":1,"title":"Opening","length":201400,"recording":{"id":"rec1"}},
			{"position":2,"title":"Closing","recording":{"id":"rec2","length":180000}}]}]}]}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// groups serves f.pages pages: every page but the last is full.
func (f *fake) groups(w http.ResponseWriter, r *http.Request) {
	var offset int
	_, _ = fmt.Sscan(r.URL.Query().Get("offset"), &offset)
	page := offset / pageSize
	n := pageSize
	if page == f.pages-1 {
		n = 1
	}
	var items []string
	for i := range n {
		items = append(items, fmt.Sprintf(`{"id":"g%d-%d","title":"Album %d","primary-type":"Album","secondary-types":["Live"],
			"first-release-date":"2020-01-02"}`, page, i, i))
	}
	if page == 0 {
		items[0] = `{"id":"` + groupMBID + `","title":"First Album","primary-type":"Album","first-release-date":"2020"}`
	}
	_, _ = fmt.Fprintf(w, `{"release-group-count":%d,"release-groups":[%s]}`, (f.pages-1)*pageSize+1, strings.Join(items, ","))
}

func (f *fake) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.reqs) }

func newClient(t *testing.T, base string, rate float64) *Client {
	t.Helper()
	p, err := New(plugin.Instance{Key: "mb", Settings: plugin.Settings{"baseUrl": base, "requestsPerSecond": rate,
		"contact": "ops@example.invalid"}, Env: conformance.Env(http.DefaultClient)})
	if err != nil {
		t.Fatal(err)
	}
	return p.(*Client)
}

func TestManifestAndErrorMapping(t *testing.T) {
	reg, ok := plugin.Default.Get(ID)
	if !ok {
		t.Fatal("musicbrainz not registered")
	}
	conformance.Manifest(t, reg.Manifest, plugin.KindMetadata)
	conformance.ErrorMapping(t, func(url string) plugin.Tester { return newClient(t, url, 50) })
}

func TestSearchArtistsMapsTheRecord(t *testing.T) {
	f := newFake(t)
	recs, err := newClient(t, f.srv.URL, 50).SearchArtists(context.Background(), "example")
	if err != nil || len(recs) != 1 {
		t.Fatalf("%v %v", recs, err)
	}
	r := recs[0]
	if r.MBID != artistMBID || r.SortName != "Artist, Example" || r.Disambiguation != "the band" || r.BeginYear != 1999 || r.Country != "GB" {
		t.Fatalf("%+v", r)
	}
	req := f.reqs[0]
	if req.URL.Query().Get("fmt") != "json" || req.Header.Get("User-Agent") != "( ops@example.invalid )" {
		t.Fatalf("request %s ua %q", req.URL, req.Header.Get("User-Agent"))
	}
}

func TestArtistPagesEveryReleaseGroup(t *testing.T) {
	f := newFake(t)
	f.pages = 2
	rec, err := newClient(t, f.srv.URL, 50).Artist(context.Background(), artistMBID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Albums) != pageSize+1 || rec.SortName != "Example Artist" {
		t.Fatalf("%d albums, sort %q", len(rec.Albums), rec.SortName)
	}
	a := rec.Albums[0]
	if a.MBID != groupMBID || a.FirstRelease == nil || a.FirstRelease.Year() != 2020 || rec.Albums[1].SecondaryTypes[0] != "Live" {
		t.Fatalf("%+v", a)
	}
	if f.count() != 3 {
		t.Fatalf("artist + 2 pages, got %d requests", f.count())
	}
}

func TestReleasesCarryMediaAndTracks(t *testing.T) {
	f := newFake(t)
	rels, err := newClient(t, f.srv.URL, 50).Releases(context.Background(), groupMBID)
	if err != nil || len(rels) != 1 {
		t.Fatalf("%v %v", rels, err)
	}
	r := rels[0]
	if r.Status != "official" || r.Date.Month() != time.March || r.TrackCount() != 2 || r.Media[0].Format != "Digital Media" {
		t.Fatalf("%+v", r)
	}
	if tr := r.Media[0].Tracks; tr[0].LengthSec != 201 || tr[1].LengthSec != 180 || tr[1].RecordingMBID != "rec2" {
		t.Fatalf("%+v", tr)
	}
	if got := f.reqs[0].URL.Query().Get("inc"); got != "media recordings" {
		t.Fatalf("inc %q", got)
	}
}

func TestNotFoundIsANormalOutcome(t *testing.T) {
	f := newFake(t)
	_, err := newClient(t, f.srv.URL, 50).Artist(context.Background(), "missing")
	if !errors.Is(err, plugin.ErrNotFound) {
		t.Fatalf("%v", err)
	}
}

func TestBusyServiceIsRetriedOnceThenReported(t *testing.T) {
	f := newFake(t)
	f.busy = 1
	c := newClient(t, f.srv.URL, 50)
	if _, err := c.SearchArtists(context.Background(), "x"); err != nil {
		t.Fatalf("one 503 is retried: %v", err)
	}
	f.busy = 2
	if _, err := c.SearchArtists(context.Background(), "x"); plugin.ClassOf(err) != plugin.ClassServerError {
		t.Fatalf("two 503s: %v", err)
	}
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("test: %v", err)
	}
}

func TestPublicServiceNeverExceedsOneRequestPerSecond(t *testing.T) {
	if Interval("musicbrainz.org", 10) != time.Second || Interval("beta.musicbrainz.org", 0) != 10*time.Second {
		t.Fatal("public host must stay at or under one request per second")
	}
	if Interval("mirror.example", 4) != 250*time.Millisecond {
		t.Fatal("a mirror uses its own rate")
	}
}

func TestQueueSpacesRequestsAcrossInstances(t *testing.T) {
	f := newFake(t)
	a, b := newClient(t, f.srv.URL, 10), newClient(t, f.srv.URL, 10)
	start := time.Now()
	for _, c := range []*Client{a, b, a} {
		if _, err := c.SearchArtists(context.Background(), "x"); err != nil {
			t.Fatal(err)
		}
	}
	if took := time.Since(start); took < 190*time.Millisecond {
		t.Fatalf("three requests at 10/s on one host took %s", took)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.SearchArtists(ctx, "x"); err == nil {
		t.Fatal("a cancelled wait fails")
	}
}

func TestSettingsAndDates(t *testing.T) {
	if _, err := New(plugin.Instance{Settings: plugin.Settings{"baseUrl": "ftp://x"}}); err == nil {
		t.Fatal("bad base url accepted")
	}
	p, err := New(plugin.Instance{Settings: plugin.Settings{"requestsPerSecond": 2}})
	if err != nil || p.(*Client).base != defaultBase || p.(*Client).interval != time.Second || !strings.Contains(p.(*Client).ua, defaultContact) {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	if Date("") != nil || Date("2020").Year() != 2020 || Date("2020-05-06").Day() != 6 {
		t.Fatal("dates")
	}
	if retryAfter(http.Header{}) != busyBackoff || retryAfter(http.Header{"Retry-After": {"999"}}) != maxRetryAfter {
		t.Fatal("retry after")
	}
}
