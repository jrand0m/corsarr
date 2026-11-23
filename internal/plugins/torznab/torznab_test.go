package torznab

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
)

const torrentBytes = "d8:announce3:foo4:infod4:name1:xee"

type fake struct {
	srv      *httptest.Server
	mu       sync.Mutex
	queries  []url.Values
	paths    []string
	emptyIDs bool // t=movie answers with no items
}

func newFake(t *testing.T) *fake {
	t.Helper()
	f := &fake{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var emptyRSS = `<rss><channel></channel></rss>`

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.queries = append(f.queries, r.URL.Query())
	f.paths = append(f.paths, r.URL.Path)
	f.mu.Unlock()
	q := r.URL.Query()
	if q.Get("apikey") != "k3y" {
		_, _ = w.Write([]byte(`<error code="100" description="Incorrect user credentials"/>`))
		return
	}
	switch {
	case r.URL.Path == "/dl/redir":
		http.Redirect(w, r, "magnet:?xt=urn:btih:2222", http.StatusFound)
	case r.URL.Path == "/dl/html":
		_, _ = w.Write([]byte("<html>login</html>"))
	case strings.HasPrefix(r.URL.Path, "/dl/") || q.Get("t") == "get":
		_, _ = w.Write([]byte(torrentBytes))
	default:
		f.api(w, q)
	}
}

func (f *fake) api(w http.ResponseWriter, q url.Values) {
	body, err := os.ReadFile("testdata/results.xml")
	if err != nil {
		w.WriteHeader(500)
		return
	}
	switch q.Get("t") {
	case "caps":
		body, _ = os.ReadFile("testdata/caps.xml")
	case "movie":
		if f.emptyIDs {
			body = []byte(emptyRSS)
		}
	}
	_, _ = w.Write(body)
}

func (f *fake) last() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries[len(f.queries)-1]
}

func (f *fake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.queries)
}

func newIndexer(t *testing.T, base string, extra plugin.Settings) *Indexer {
	t.Helper()
	s := plugin.Settings{"url": base, "apiKey": "k3y", "priority": 10}
	for k, v := range extra {
		s[k] = v
	}
	p, err := New(plugin.Instance{Key: "fixture", Name: "Fixture", PluginID: ID, Settings: s,
		Env: conformance.Env(http.DefaultClient)})
	if err != nil {
		t.Fatal(err)
	}
	return p.(*Indexer)
}

func movieReq() domain.SearchRequest {
	return domain.SearchRequest{Kind: domain.KindMovie, Mode: domain.SearchMovie, Text: "Some Movie", Year: 2021,
		IDs: map[string]string{"imdb": "tt1234567", "tmdb": "603"}, Origin: domain.OriginInteractive}
}

func TestConformance(t *testing.T) {
	reg, ok := plugin.Default.Get(ID)
	if !ok {
		t.Fatal("torznab not registered")
	}
	conformance.Manifest(t, reg.Manifest, plugin.KindIndexer)
	f := newFake(t)
	conformance.Indexer(t, newIndexer(t, f.srv.URL, nil), movieReq())
}

func TestErrorMapping(t *testing.T) {
	conformance.ErrorMapping(t, func(url string) plugin.Tester { return newIndexer(t, url, nil) },
		conformance.ErrorCase{Name: "garbage", Want: plugin.ClassParse,
			Handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }},
		conformance.ErrorCase{Name: "no modes", Want: plugin.ClassParse,
			Handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<caps><searching/></caps>")) }},
		conformance.ErrorCase{Name: "bad key", Want: plugin.ClassAuth,
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`<error code="101" description="suspended"/>`))
			}},
		conformance.ErrorCase{Name: "request limit", Want: plugin.ClassRateLimited,
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`<error code="500" description="limit"/>`))
			}},
		conformance.ErrorCase{Name: "bad param", Want: plugin.ClassConfig,
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`<error code="201" description="param"/>`))
			}},
		conformance.ErrorCase{Name: "other error", Want: plugin.ClassServerError,
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`<error code="900" description="x"/>`))
			}},
	)
}

func TestCapsParsed(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	c, err := ix.Caps(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.SearchMode{domain.SearchText, domain.SearchTV, domain.SearchMovie}
	if len(c.Modes) != 3 || c.Modes[2] != want[2] || c.Limit != 100 {
		t.Errorf("caps %+v", c)
	}
	if c.Categories[2040] != "Movies/HD" || len(c.IDParams) != 3 {
		t.Errorf("categories/ids %+v", c)
	}
	n := f.count()
	if _, err := ix.Caps(t.Context()); err != nil || f.count() != n {
		t.Error("caps must be cached")
	}
}

func TestSearchUsesIDQuery(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	rels, err := ix.Search(t.Context(), movieReq())
	if err != nil || len(rels) != 2 {
		t.Fatalf("%v %d", err, len(rels))
	}
	q := f.last()
	if q.Get("t") != "movie" || q.Get("imdbid") != "1234567" || q.Get("tmdbid") != "603" || q.Get("cat") != "2000" || q.Get("q") != "" {
		t.Errorf("query %v", q)
	}
}

func TestSearchFallsBackToText(t *testing.T) {
	f := newFake(t)
	f.emptyIDs = true
	ix := newIndexer(t, f.srv.URL, nil)
	rels, err := ix.Search(t.Context(), movieReq())
	if err != nil || len(rels) != 2 {
		t.Fatalf("%v %d", err, len(rels))
	}
	if q := f.last(); q.Get("t") != "search" || q.Get("q") != "Some Movie 2021" {
		t.Errorf("fallback query %v", q)
	}
}

func TestSearchTextOnlyWhenNoIDs(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	req := domain.SearchRequest{Kind: domain.KindMovie, Mode: domain.SearchMovie, Titles: []string{"Some Movie 2021"}, Year: 2021}
	if _, err := ix.Search(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if q := f.last(); q.Get("t") != "search" || q.Get("q") != "Some Movie 2021" {
		t.Errorf("year must not be duplicated: %v", q)
	}
	req = domain.SearchRequest{Kind: domain.KindMovie, Mode: domain.SearchMovie}
	rels, err := ix.Search(t.Context(), req)
	if err != nil || rels != nil {
		t.Errorf("nothing to search with: %v %v", rels, err)
	}
}

func TestSearchTVQuery(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	req := domain.SearchRequest{Kind: domain.KindTV, Mode: domain.SearchTV, IDs: map[string]string{"tvdb": "81189", "tmdb": "1"}, Season: 2, Episodes: []int{5}}
	if _, err := ix.Search(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	q := f.last()
	if q.Get("t") != "tvsearch" || q.Get("tvdbid") != "81189" || q.Get("season") != "2" || q.Get("ep") != "5" || q.Get("tmdbid") != "" || q.Get("cat") != "5000" {
		t.Errorf("tv query %v", q)
	}
}

func TestSearchModeNotAdvertised(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	req := domain.SearchRequest{Kind: domain.KindMusic, Mode: domain.SearchMusic, Text: "Album", IDs: map[string]string{"x": "1"}}
	if _, err := ix.Search(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if q := f.last(); q.Get("t") != "search" || q.Get("cat") != "3000" {
		t.Errorf("music falls back to text with kind categories: %v", q)
	}
}

func TestReleaseMapping(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	rels, err := ix.Search(t.Context(), movieReq())
	if err != nil {
		t.Fatal(err)
	}
	r := rels[0]
	if r.ID != "guid-1" || r.Title != "Some.Movie.2021.1080p.BluRay.x264-GRP" || r.Protocol != domain.ProtoTorrent {
		t.Errorf("identity %+v", r)
	}
	if r.Size != 2147483648 || r.Seeders != 42 || r.Peers != 50 || r.Grabs != 7 || r.InfoHash != "abcdef0123456789abcdef0123456789abcdef01" {
		t.Errorf("numbers %+v", r)
	}
	if len(r.Flags) != 1 || r.Flags[0] != "freeleech" {
		t.Errorf("flags %v", r.Flags)
	}
	if got := r.Categories; len(got) != 2 || got[0] != 2040 || got[1] != 2000 {
		t.Errorf("categories %v", got)
	}
	if r.ExternalIDs["imdb"] != "tt1234567" || r.ExternalIDs["tmdb"] != "603" || len(r.ExternalIDs) != 2 {
		t.Errorf("ids %v", r.ExternalIDs)
	}
	if !r.PublishedAt.Equal(time.Date(2021, 11, 15, 10, 0, 0, 0, time.UTC)) || r.DetailsURL != "https://tracker.example/details/1" {
		t.Errorf("when/details %+v", r)
	}
	if r.Indexers[0].Key != "fixture" || r.Indexers[0].Priority != 10 {
		t.Errorf("indexer ref %+v", r.Indexers)
	}
	second := rels[1]
	if second.Size != 734003200 || len(second.Flags) != 0 || second.ExternalIDs != nil {
		t.Errorf("second %+v", second)
	}
}

func TestFeedCursor(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	rels, cur, err := ix.Feed(t.Context(), domain.KindMovie, "")
	if err != nil || len(rels) != 2 || cur != "2021-11-15T10:00:00Z|guid-1" {
		t.Fatalf("first: %v %d %q", err, len(rels), cur)
	}
	rels, cur2, err := ix.Feed(t.Context(), domain.KindMovie, cur)
	if err != nil || len(rels) != 0 || cur2 != cur {
		t.Fatalf("again: %v %d %q", err, len(rels), cur2)
	}
	older := "2021-11-14T12:00:00Z|x"
	rels, cur3, _ := ix.Feed(t.Context(), domain.KindMovie, older)
	if len(rels) != 1 || rels[0].ID != "guid-1" || cur3 != cur {
		t.Fatalf("partial: %d %q", len(rels), cur3)
	}
	tie := "2021-11-15T10:00:00Z|other"
	if rels, _, _ := ix.Feed(t.Context(), domain.KindMovie, tie); len(rels) != 1 {
		t.Errorf("same-time different guid is new: %d", len(rels))
	}
}

func TestFeedNoDatesKeepsCursor(t *testing.T) {
	rels, cur, _ := newerThan(nil, "c")
	if rels != nil || cur != "c" {
		t.Fatal("empty feed keeps the cursor")
	}
}

func TestFeedErrorKeepsCursor(t *testing.T) {
	ix := newIndexer(t, "http://127.0.0.1:1", nil)
	_, cur, err := ix.Feed(t.Context(), domain.KindMovie, "keep")
	if err == nil || cur != "keep" {
		t.Fatal("failed feed must not advance the cursor")
	}
}

func resolveFirst(t *testing.T, ix *Indexer, i int) (plugin.Payload, error) {
	t.Helper()
	rels, err := ix.Search(t.Context(), movieReq())
	if err != nil {
		t.Fatal(err)
	}
	return ix.Resolve(t.Context(), rels[i])
}

func TestResolveTorrentUsesAPIKey(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	p, err := resolveFirst(t, ix, 0)
	if err != nil || p.Kind != "torrent" || string(p.Content) != torrentBytes || p.Name == "" {
		t.Fatalf("%+v %v", p, err)
	}
	if q := f.last(); q.Get("apikey") != "k3y" {
		t.Errorf("api key missing on download: %v", q)
	}
}

func TestResolveMagnetPassthrough(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	n := 0
	p, err := resolveFirst(t, ix, 1)
	n = f.count()
	if err != nil || p.Kind != "magnet" || !strings.HasPrefix(p.URL, "magnet:?xt=urn:btih:1111") {
		t.Fatalf("%+v %v", p, err)
	}
	if f.count() != n {
		t.Error("magnet must not fetch")
	}
}

func TestResolveMagnetAttrWhenNoLink(t *testing.T) {
	ix := newIndexer(t, "http://example.invalid", nil)
	ix.links.put("r", target{magnet: "magnet:?xt=urn:btih:3333"})
	p, err := ix.Resolve(t.Context(), domain.Release{ID: "r"})
	if err != nil || p.Kind != "magnet" {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestResolveRedirectToMagnet(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	ix.links.put("r", target{link: "/dl/redir"})
	p, err := ix.Resolve(t.Context(), domain.Release{ID: "r", Title: "T"})
	if err != nil || p.Kind != "magnet" || !strings.HasPrefix(p.URL, "magnet:?xt=urn:btih:2222") {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestResolveNotATorrent(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	ix.links.put("r", target{link: "/dl/html"})
	if _, err := ix.Resolve(t.Context(), domain.Release{ID: "r"}); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("html download: %v", err)
	}
}

func TestResolveAfterRestartUsesGet(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	p, err := ix.Resolve(t.Context(), domain.Release{ID: "guid-9", GUID: "guid-9"})
	if err != nil || p.Kind != "torrent" {
		t.Fatalf("%+v %v", p, err)
	}
	if q := f.last(); q.Get("t") != "get" || q.Get("id") != "guid-9" || q.Get("apikey") != "k3y" {
		t.Errorf("get query %v", q)
	}
	if _, err := ix.Resolve(t.Context(), domain.Release{ID: "x"}); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("unknown release without guid: %v", err)
	}
}

func TestResolveAfterRestartUsesPersistedDownloadURL(t *testing.T) {
	f := newFake(t)
	ix := newIndexer(t, f.srv.URL, nil)
	rels, err := ix.Search(t.Context(), movieReq())
	if err != nil || rels[1].DownloadURL == "" || !strings.HasPrefix(rels[1].DownloadURL, "magnet:") {
		t.Fatalf("download url not carried: %+v %v", rels, err)
	}
	fresh := newIndexer(t, f.srv.URL, nil) // a restarted process has an empty link cache
	p, err := fresh.Resolve(t.Context(), domain.Release{ID: "gone", DownloadURL: rels[1].DownloadURL})
	if err != nil || p.Kind != "magnet" || p.URL != rels[1].DownloadURL {
		t.Fatalf("%+v %v", p, err)
	}
	if b, _ := json.Marshal(rels[0]); strings.Contains(string(b), rels[0].DownloadURL) {
		t.Fatalf("download url leaked into JSON: %s", b)
	}
}

func TestAPIKeyNotSentToOtherHosts(t *testing.T) {
	ix := newIndexer(t, "http://indexer.test", nil)
	got := ix.authorize("http://cdn.example/x.torrent")
	if strings.Contains(got, "apikey") {
		t.Errorf("leaked key: %s", got)
	}
	if got := ix.authorize("/dl/1?apikey=own"); !strings.Contains(got, "apikey=own") || strings.Contains(got, "k3y") {
		t.Errorf("existing key kept: %s", got)
	}
	if got := ix.authorize("http://[bad"); got != "http://[bad" {
		t.Errorf("unparseable link returned as is: %s", got)
	}
}

func TestLinkCacheBounded(t *testing.T) {
	c := newLinkCache(2)
	c.put("a", target{link: "1"})
	c.put("b", target{link: "2"})
	c.put("b", target{link: "2b"})
	c.put("c", target{link: "3"})
	if _, ok := c.get("a"); ok {
		t.Error("oldest evicted")
	}
	if tg, ok := c.get("b"); !ok || tg.link != "2b" {
		t.Error("update in place")
	}
}

func TestSettingsValidation(t *testing.T) {
	bad := []plugin.Settings{
		{"url": ""}, {"url": "ftp://x"}, {"url": "http://x", "categories": map[string]any{"podcasts": []any{1}}},
		{"url": "http://x", "categories": map[string]any{"movie": []any{"abc"}}},
		{"url": "http://x", "categories": map[string]any{"movie": 5}},
	}
	for i, s := range bad {
		if _, err := New(plugin.Instance{Settings: s}); plugin.ClassOf(err) != plugin.ClassConfig {
			t.Errorf("case %d: %v", i, err)
		}
	}
}

func TestCategoryParsing(t *testing.T) {
	ix := newIndexer(t, "http://x", plugin.Settings{"categories": map[string]any{
		"movies": "2000, 2010", "tv": []any{5030, float64(5040), int64(5045)}, "audio": []int{3010}}})
	if joinInts(ix.cats[domain.KindMovie]) != "2000,2010" || joinInts(ix.cats[domain.KindTV]) != "5030,5040,5045" ||
		joinInts(ix.cats[domain.KindMusic]) != "3010" || joinInts(ix.cats[domain.KindBook]) != "7000" {
		t.Errorf("%v", ix.cats)
	}
}

func TestEndpointPath(t *testing.T) {
	cases := map[string]plugin.Settings{
		"http://h/api":         {"url": "http://h"},
		"http://h/x/api":       {"url": "http://h/x/"},
		"http://h/torznab/all": {"url": "http://h/torznab/all", "apiPath": ""},
		"http://h/custom":      {"url": "http://h", "apiPath": "/custom"},
	}
	for want, s := range cases {
		u, err := parseEndpoint(s)
		if err != nil || u.String() != want {
			t.Errorf("%v → %v (%v), want %s", s, u, err, want)
		}
	}
}

func TestRSSOddities(t *testing.T) {
	if _, err := parseRSS([]byte("<rss><channel><item><title>x")); plugin.ClassOf(err) != plugin.ClassParse {
		t.Error("truncated xml")
	}
	if toString(true) != "" {
		t.Error("toString unknown")
	}
	if !parseDate("garbage").IsZero() || parseDate("Mon, 2 Jan 2006 15:04:05 +0000").IsZero() {
		t.Error("dates")
	}
	it := rssItem{Enclosure: struct {
		URL    string `xml:"url,attr"`
		Length string `xml:"length,attr"`
	}{Length: "99"}}
	if it.size() != 99 {
		t.Error("enclosure length size")
	}
}
