package declarative

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jrand0m/corsarr/internal/definition"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
)

// rt sends requests without following redirects, like the shared client.
type rt struct{}

func (rt) Do(r *http.Request) (*http.Response, error) { return http.DefaultTransport.RoundTrip(r) }

type env struct{ now time.Time }

var fixedNow = time.Date(2026, 6, 1, 12, 34, 0, 0, time.UTC)

func (env) HTTP() plugin.HTTPDoer { return rt{} }
func (e env) Now() time.Time {
	if e.now.IsZero() {
		return time.Now()
	}
	return e.now
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func bundled(t *testing.T, id string) *definition.Definition {
	t.Helper()
	d, ok := Default.Definition(id)
	if !ok {
		t.Fatalf("%s not bundled", id)
	}
	return d
}

func instance(t *testing.T, d *definition.Definition, base string, extra plugin.Settings) *Indexer {
	t.Helper()
	s := plugin.Settings{"baseUrl": base}
	for k, v := range extra {
		s[k] = v
	}
	ix, err := New(d, plugin.Instance{Key: domain.ObjectKey(d.ID), Name: d.Name, PluginID: d.ID, Settings: s, Env: env{now: fixedNow}})
	if err != nil {
		t.Fatal(err)
	}
	ix.pace.every = 0
	return ix
}

func TestKnabenSearchesTheJSONAPI(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1" || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, 400)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		_, _ = w.Write(fixture(t, "knaben.json"))
	}))
	defer srv.Close()
	ix := instance(t, bundled(t, "knaben"), srv.URL, nil)
	req := domain.SearchRequest{Kind: domain.KindTV, Mode: domain.SearchTV, Text: "Some Show", Season: 1, Episodes: []int{2}}
	conformance.Indexer(t, ix, req)
	rels, err := ix.Search(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got["query"] != "Some Show S01E02" || got["hide_unsafe"] != true || len(got["categories"].([]any)) != 1 {
		t.Errorf("request body %v", got)
	}
	r := rels[1]
	if r.InfoHash != "1111222233334444555566667777888899990000" || r.Size != 1503238553 || r.Seeders != 120 || r.Peers != 15 ||
		r.Categories[0] != 5000 || r.PublishedAt.Day() != 4 || !strings.HasPrefix(r.DownloadURL, "magnet:") || r.Indexers[0].Key != "knaben" {
		t.Errorf("release %+v", r)
	}
	if rels[0].Categories[0] != 3040 {
		t.Errorf("lossless category %v", rels[0].Categories)
	}
}

func TestPirateBayIDSearchFallsBackToText(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		if r.URL.Query().Get("q") == "nothing here" {
			_, _ = w.Write(fixture(t, "apibay_empty.json"))
			return
		}
		_, _ = w.Write(fixture(t, "apibay.json"))
	}))
	defer srv.Close()
	ix := instance(t, bundled(t, "thepiratebay"), srv.URL, nil)
	movie := domain.SearchRequest{Kind: domain.KindMovie, Mode: domain.SearchMovie, Titles: []string{"Example Movie"}, Year: 1994,
		IDs: map[string]string{"imdb": "tt0111161"}}
	conformance.Indexer(t, ix, movie)
	rels, err := ix.Search(context.Background(), movie)
	if err != nil || len(rels) != 2 {
		t.Fatalf("%v %v", rels, err)
	}
	if !strings.Contains(queries[len(queries)-1], "q=tt0111161") || !strings.Contains(queries[len(queries)-1], "cat=201%2C207%2C211") {
		t.Errorf("id query %q", queries[len(queries)-1])
	}
	r := rels[0]
	if r.Categories[0] != 2040 || r.ExternalIDs["imdb"] != "tt0111161" || r.Size != 2<<30 || r.Peers != 259 ||
		r.DetailsURL != "https://thepiratebay.org/description.php?id=5501" || !strings.Contains(r.DownloadURL, "tr=udp") {
		t.Errorf("release %+v", r)
	}
	none, err := ix.Search(context.Background(), domain.SearchRequest{Mode: domain.SearchText, Text: "nothing here"})
	if err != nil || len(none) != 0 {
		t.Errorf("placeholder row not skipped: %v %v", none, err)
	}
	tv, _ := ix.Search(context.Background(), domain.SearchRequest{Kind: domain.KindTV, Mode: domain.SearchTV, Text: "Show", Season: 2})
	if q := queries[len(queries)-1]; !strings.Contains(q, "q=Show+S02") || len(tv) != 2 {
		t.Errorf("tv text query %q", q)
	}
}

// site fakes an HTML tracker behind a form login.
type site struct {
	t         *testing.T
	logins    atomic.Int32
	expireOne atomic.Bool
	srv       *httptest.Server
}

func newSite(t *testing.T) *site {
	s := &site{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /takelogin.php", s.login)
	mux.HandleFunc("GET /index.php", s.authed(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<a href="/logout.php">Log out</a>`))
	}))
	mux.HandleFunc("GET /browse.php", s.authed(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("search") == "broken" {
			_, _ = w.Write([]byte(`<html><body><p>new layout</p></body></html>`))
			return
		}
		_, _ = w.Write(fixture(t, "browse.html"))
	}))
	mux.HandleFunc("GET /details.php", s.authed(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(fixture(t, "details.html")) }))
	mux.HandleFunc("GET /download.php", s.authed(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("d8:announce0:e")) }))
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *site) login(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if r.PostForm.Get("user") != "owner" || r.PostForm.Get("pass") != "pw" {
		_, _ = w.Write([]byte(`<div class="login-error">Username or password incorrect</div>`))
		return
	}
	s.logins.Add(1)
	http.SetCookie(w, &http.Cookie{Name: "sid", Value: "s" + string(rune('0'+s.logins.Load()))})
	http.Redirect(w, r, "/index.php", http.StatusFound)
}

func (s *site) authed(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("sid")
		if err != nil || (s.expireOne.Load() && r.URL.Path == "/browse.php") {
			s.expireOne.Store(false)
			http.Redirect(w, r, "/login.php", http.StatusFound)
			return
		}
		_ = c
		h(w, r)
	}
}

func exampleDef(t *testing.T) *definition.Definition {
	d, err := definition.Parse(fixture(t, "examplesite.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestHTMLSiteWithFormLogin(t *testing.T) {
	s := newSite(t)
	ix := instance(t, exampleDef(t), s.srv.URL, plugin.Settings{"username": "owner", "password": "pw"})
	conformance.Indexer(t, ix, domain.SearchRequest{Kind: domain.KindMusic, Mode: domain.SearchMusic, Text: "Quiet Album"})
	rels, err := ix.Search(context.Background(), domain.SearchRequest{Kind: domain.KindMusic, Mode: domain.SearchMusic, Text: "Quiet"})
	if err != nil || len(rels) != 2 {
		t.Fatalf("%v %v", rels, err)
	}
	r := rels[0]
	if r.Title != "Quiet Album 2023 24bit FLAC" || r.Size != 1288490188 || r.Seeders != 33 || r.Peers != 37 || r.Grabs != 1024 ||
		r.Categories[0] != 3040 || r.PublishedAt.Year() != 2025 || r.Flags[0] != "freeleech" || r.DetailsURL != s.srv.URL+"/details.php?id=901" {
		t.Errorf("release %+v", r)
	}
	if s.logins.Load() != 1 {
		t.Errorf("logged in %d times, want once for the whole session", s.logins.Load())
	}
	p, err := ix.Resolve(context.Background(), r)
	if err != nil || p.Kind != "torrent" || string(p.Content) != "d8:announce0:e" {
		t.Fatalf("resolve %+v %v", p, err)
	}
	s.expireOne.Store(true)
	if _, err := ix.Search(context.Background(), domain.SearchRequest{Mode: domain.SearchText, Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if s.logins.Load() != 2 {
		t.Errorf("expired session: %d logins, want exactly one re-login", s.logins.Load())
	}
	_, err = ix.Search(context.Background(), domain.SearchRequest{Mode: domain.SearchText, Text: "broken"})
	if plugin.ClassOf(err) != plugin.ClassParse || !strings.Contains(err.Error(), "could not be parsed") {
		t.Errorf("layout change: %v", err)
	}
}

func TestFormLoginFailuresAreAuth(t *testing.T) {
	s := newSite(t)
	ix := instance(t, exampleDef(t), s.srv.URL, plugin.Settings{"username": "owner", "password": "wrong"})
	err := ix.Test(context.Background())
	if plugin.ClassOf(err) != plugin.ClassAuth || !strings.Contains(err.Error(), "Username or password incorrect") {
		t.Errorf("%v", err)
	}
}

func TestCookieLoginExpiryNeedsTheOwner(t *testing.T) {
	d := exampleDef(t)
	d.Login = definition.Login{Method: definition.LoginCookie, Expired: definition.Check{Redirect: "login.php"}}
	if _, err := New(d, plugin.Instance{Settings: plugin.Settings{}, Env: env{}}); plugin.ClassOf(err) != plugin.ClassConfig {
		t.Errorf("missing cookie: %v", err)
	}
	s := newSite(t)
	ix := instance(t, d, s.srv.URL, plugin.Settings{"cookie": "sid=pasted"})
	if _, err := ix.Search(context.Background(), domain.SearchRequest{Mode: domain.SearchText, Text: "x"}); err != nil {
		t.Fatalf("pasted cookie rejected: %v", err)
	}
	s.expireOne.Store(true)
	_, err := ix.Search(context.Background(), domain.SearchRequest{Mode: domain.SearchText, Text: "x"})
	if plugin.ClassOf(err) != plugin.ClassSessionExpired {
		t.Errorf("%v", err)
	}
}

func TestErrorsAndEdgeCases(t *testing.T) {
	status := http.StatusOK
	body := `[]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567", http.StatusFound)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	d := bundled(t, "thepiratebay")
	ix := instance(t, d, srv.URL, nil)
	ctx := context.Background()
	for code, class := range map[int]plugin.FailureClass{401: plugin.ClassAuth, 429: plugin.ClassRateLimited, 503: plugin.ClassServerError} {
		status = code
		if _, err := ix.Search(ctx, domain.SearchRequest{Mode: domain.SearchText, Text: "x"}); plugin.ClassOf(err) != class {
			t.Errorf("HTTP %d: %v", code, err)
		}
	}
	status, body = http.StatusOK, `not json`
	if _, err := ix.Search(ctx, domain.SearchRequest{Mode: domain.SearchText, Text: "x"}); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("bad json: %v", err)
	}
	if err := ix.Test(ctx); err == nil {
		t.Error("self-test passed on garbage")
	}
	body = `[]`
	if err := ix.Test(ctx); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("self-test with no rows: %v", err)
	}
	p, err := ix.Resolve(ctx, domain.Release{ID: "x", Title: "T", DownloadURL: srv.URL + "/redirect"})
	if err != nil || p.Kind != "magnet" {
		t.Errorf("redirect to magnet: %+v %v", p, err)
	}
	if p, err := ix.Resolve(ctx, domain.Release{ID: "m", DownloadURL: "magnet:?xt=x"}); err != nil || p.URL != "magnet:?xt=x" {
		t.Errorf("magnet passthrough %v", err)
	}
	if _, err := ix.Resolve(ctx, domain.Release{ID: "none"}); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("no link: %v", err)
	}
	body = `<html>`
	if _, err := ix.Resolve(ctx, domain.Release{ID: "y", DownloadURL: srv.URL + "/file"}); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("not a torrent: %v", err)
	}
	status = 403
	if _, err := ix.Resolve(ctx, domain.Release{ID: "y", DownloadURL: srv.URL + "/file"}); plugin.ClassOf(err) != plugin.ClassAuth {
		t.Errorf("forbidden download: %v", err)
	}
	if _, err := New(d, plugin.Instance{Settings: plugin.Settings{"baseUrl": "ftp://x"}}); plugin.ClassOf(err) != plugin.ClassConfig {
		t.Error("bad base accepted")
	}
	d2 := *d
	d2.Search.Requests = d.Search.Requests[:1]
	ix2 := instance(t, &d2, srv.URL, nil)
	if _, err := ix2.Search(ctx, domain.SearchRequest{Mode: domain.SearchText, Text: "x"}); plugin.ClassOf(err) != plugin.ClassConfig {
		t.Errorf("no request: %v", err)
	}
	if host, every := ix.MinInterval(); host != "127.0.0.1" || every != 0 || ix.Definition() != d {
		t.Error(host, every)
	}
}

func TestFeedCursorAndCategorySearch(t *testing.T) {
	var lastCat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastCat = r.URL.Query().Get("cat")
		_, _ = w.Write(fixture(t, "apibay.json"))
	}))
	defer srv.Close()
	ix := instance(t, bundled(t, "thepiratebay"), srv.URL, nil)
	ctx := context.Background()
	rels, cur, err := ix.Feed(ctx, domain.KindMovie, "")
	if err != nil || len(rels) != 2 || cur == "" {
		t.Fatal(rels, cur, err)
	}
	again, cur2, _ := ix.Feed(ctx, domain.KindMovie, cur)
	if len(again) != 0 || cur2 != cur {
		t.Errorf("replayed feed handed %d", len(again))
	}
	if _, err := ix.SearchCategories(ctx, domain.SearchRequest{Mode: domain.SearchText, Text: "x"}, []int{5000}); err != nil || lastCat != "205,208,212" {
		t.Errorf("cat %q %v", lastCat, err)
	}
	if _, err := ix.SearchCategories(ctx, domain.SearchRequest{Kind: domain.KindBook, Mode: domain.SearchText, Text: "x"}, nil); err != nil || lastCat != "601" {
		t.Errorf("cat %q", lastCat)
	}
}

func TestPacerSpacesRequests(t *testing.T) {
	p := &pacer{every: 20 * time.Millisecond}
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := p.wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Error("not spaced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.next = time.Now().Add(time.Hour)
	if p.wait(ctx) == nil {
		t.Error("cancel ignored")
	}
}

func TestManifestAndCaps(t *testing.T) {
	m := Manifest(exampleDef(t))
	if m.Delivery != Delivery || m.SetupFlows[0] != plugin.FlowLoginForm || m.Settings[0].Key != "baseUrl" {
		t.Errorf("%+v", m)
	}
	conformance.Manifest(t, m, plugin.KindIndexer)
	d := exampleDef(t)
	d.Login.Method = definition.LoginCookie
	d.Links = append(d.Links, "https://mirror.test/")
	m = Manifest(d)
	if m.SetupFlows[0] != plugin.FlowCookiePaste || !strings.Contains(m.Settings[0].Help, "mirror.test") {
		t.Errorf("%+v", m)
	}
	caps := CapsOf(bundled(t, "thepiratebay"))
	if len(caps.Modes) != 5 || caps.IDParams[0] != "imdbid" || caps.Categories[2040] != "Movies/HD" {
		t.Errorf("%+v", caps)
	}
	if settingText(nil, nil) != "" || settingText(true, nil) != "true" || settingText(nil, "d") != "d" {
		t.Error("settingText")
	}
}

func TestCatalogLoadsOverridesAndDrops(t *testing.T) {
	reg := plugin.NewRegistry()
	_ = reg.Register(plugin.Registration{Manifest: plugin.Manifest{ID: "torznab", Kind: plugin.KindIndexer, APIVersion: plugin.APIVersion, Delivery: "builtin"},
		New: func(plugin.Instance) (any, error) { return nil, nil }})
	knaben, _ := os.ReadFile("bundled/knaben.yaml")
	c, err := NewCatalog(reg, fstest.MapFS{"bundled/knaben.yaml": {Data: knaben}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(name, data string) { _ = os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600) }
	write("knaben.yaml", strings.Replace(string(knaben), "name: Knaben", "name: Knaben Local", 1))
	write("examplesite.yml", string(fixture(t, "examplesite.yaml")))
	write("broken.json", `{"schema": 1, "id": "broken"}`)
	write("dup.yaml", string(fixture(t, "examplesite.yaml")))
	write("torznab.yaml", strings.Replace(string(knaben), "id: knaben", "id: torznab", 1))
	write("notes.txt", "ignored")
	entries := c.Load(dir)
	byFile := map[string]Entry{}
	for _, e := range entries {
		byFile[e.Origin+"/"+e.File] = e
	}
	if e := byFile["user/knaben.yaml"]; !e.Valid || !e.Overrides {
		t.Errorf("override %+v", e)
	}
	if e := byFile["user/broken.json"]; e.Valid || len(e.Problems) == 0 {
		t.Errorf("broken %+v", e)
	}
	if e := byFile["user/examplesite.yml"]; !e.Valid && !byFile["user/dup.yaml"].Valid {
		t.Error("both duplicates invalid")
	}
	if got, _ := reg.Get("knaben"); got.Manifest.Name != "Knaben Local" {
		t.Errorf("user file did not override: %s", got.Manifest.Name)
	}
	if got, _ := reg.Get("torznab"); got.Manifest.Delivery != "builtin" {
		t.Error("user file shadowed a built-in")
	}
	for _, f := range []string{"knaben.yaml", "examplesite.yml", "dup.yaml"} {
		_ = os.Remove(filepath.Join(dir, f))
	}
	c.Load(dir)
	if got, _ := reg.Get("knaben"); got.Manifest.Name != "Knaben" {
		t.Error("bundled not restored")
	}
	if _, ok := reg.Get("examplesite"); ok {
		t.Error("removed user definition still registered")
	}
	if _, ok := c.Definition("examplesite"); ok || len(c.Entries()) == 0 {
		t.Error("catalogue out of step")
	}
	if c.Load(filepath.Join(dir, "missing")) == nil {
		t.Error("missing dir lost bundled entries")
	}
	if _, err := NewCatalog(reg, fstest.MapFS{"bundled/x.yaml": {Data: []byte("schema: 9")}}); err == nil {
		t.Error("broken bundled accepted")
	}
}

func TestBundledDefinitionsAreRegistered(t *testing.T) {
	for _, id := range []string{"knaben", "thepiratebay"} {
		reg, ok := plugin.Default.Get(id)
		if !ok || reg.Manifest.Delivery != Delivery {
			t.Errorf("%s not registered", id)
			continue
		}
		conformance.Manifest(t, reg.Manifest, plugin.KindIndexer)
		impl, err := reg.New(plugin.Instance{Key: domain.ObjectKey(id), Settings: plugin.Settings{}, Env: env{}})
		if _, isIx := impl.(plugin.Indexer); err != nil || !isIx {
			t.Errorf("%s factory: %v", id, err)
		}
	}
}
