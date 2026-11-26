// Package e2e proves the movie pipeline end to end against fakes on httptest — no internet: add a movie → search on add →
// decision → grab → qBittorrent completion → hardlink import → Plex refresh
// and one ntfy message → History; and the unsafe-payload path.
package e2e

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/app"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/events"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/httpx"
	"github.com/jrand0m/corsarr/internal/plugin"
	_ "github.com/jrand0m/corsarr/internal/plugins" // built-in plugins
	"github.com/jrand0m/corsarr/internal/scheduler"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/testkit"
)

type stack struct {
	api     *httptest.Server
	key     string
	library string
	shelf   string // a second movie root folder
	shows   string // TV root folder
	dl      string
	tz      *fakeTorznab
	tmdb    *httptest.Server
	qb      *fakeQB
	plex    *fakePlex
	ntfy    *fakeNtfy
	yaml    string
	path    string                        // config file
	cfg     atomic.Pointer[config.Config] // what Config() returns, swapped on reload
	app     *app.App
}

const configYAML = `version: 1
mediaKinds:
  movie: { mode: {{mode}} }
  tv: { mode: {{tvmode}} }
data:
  dir: {{data}}
rootFolders:
  - path: {{library}}
    kind: movie
  - path: {{shelf}}
    kind: movie
  - path: {{shows}}
    kind: tv
legacy:
  sonarr: { enabled: true }
  radarr: { enabled: true }
  lidarr: { enabled: true }
  prowlarr: { enabled: true }
metadata:
  tmdb:
    type: tmdb
    apiKey: tmdb-key
    baseUrl: {{tmdb}}
indexers:
  fake:
    type: torznab
    url: {{torznab}}
    apiKey: tz-key
downloadClients:
  qbit:
    type: qbittorrent
    url: {{qbit}}
    username: admin
    password: pw
mediaServers:
  plex:
    type: plex
    url: {{plex}}
    token: plex-token
notifications:
  phone:
    type: native
    urls: ["ntfy://{{ntfy}}/corsarr?scheme=http"]
    on: [import]
  both:
    type: native
    urls: ["ntfy://{{ntfy}}/both?scheme=http"]
    on: [grab, import]
    template:
      title: "[corsarr] {{.Title}}"
`

// start builds the fakes, writes the YAML config, loads it like `serve` does
// and wires the kernel and pipeline with short timings.
func start(t *testing.T, payload []string) *stack { return startIn(t, domain.ModeActive, payload) }

func startIn(t *testing.T, mode domain.KindMode, payload []string) *stack {
	return startKinds(t, mode, domain.ModeActive, payload)
}

func startKinds(t *testing.T, mode, tvMode domain.KindMode, payload []string) *stack {
	t.Helper()
	base := t.TempDir()
	s := &stack{library: filepath.Join(base, "media", "movies"), shelf: filepath.Join(base, "shelf", "movies"),
		shows: filepath.Join(base, "media", "tv"), dl: filepath.Join(base, "downloads")}
	for _, d := range []string{s.library, s.shelf, s.shows, s.dl} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s.tz, s.qb, s.plex, s.ntfy, s.tmdb = newTorznab(t), newQB(t, s.dl, payload), newPlex(t, s.shows, s.library, s.shelf), newNtfy(t), fakeTMDB(t)
	ntfyURL, _ := url.Parse(s.ntfy.srv.URL)
	s.yaml = strings.NewReplacer("{{data}}", filepath.Join(base, "data"), "{{library}}", s.library, "{{shelf}}", s.shelf, "{{tmdb}}", s.tmdb.URL,
		"{{torznab}}", s.tz.srv.URL, "{{qbit}}", s.qb.srv.URL, "{{plex}}", s.plex.srv.URL, "{{ntfy}}", ntfyURL.Host, "{{mode}}", string(mode),
		"{{shows}}", s.shows, "{{tvmode}}", string(tvMode)).Replace(configYAML)
	s.path = filepath.Join(base, "corsarr.yml")
	s.cfg.Store(s.load(t, s.yaml))
	s.wire(t)
	return s
}

// load writes the YAML and loads it like `serve` does.
func (s *stack) load(t *testing.T, yaml string) *config.Config {
	t.Helper()
	if err := os.WriteFile(s.path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.OSInputs([]string{"--configFile=" + s.path}), plugin.Default)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// setMode changes the movie mode as a watched-file reload does.
func (s *stack) setMode(t *testing.T, from, to domain.KindMode) {
	cfg := s.load(t, strings.Replace(s.yaml, "mode: "+string(from), "mode: "+string(to), 1))
	s.cfg.Store(cfg)
	s.app.OnConfig(cfg)
}

func (s *stack) wire(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(ctx, s.cfg.Load().Data.Dir)
	if err != nil {
		t.Fatal(err)
	}
	bus := events.New(events.WithHistory(st.History(), nil))
	reg := health.New(bus, st.Health(), nil)
	client := httpx.New(httpx.Options{Reporter: reg, Default: httpx.Limits{Rate: 1000, Burst: 1000}, MaxRetries: -1})
	jobs := scheduler.New(scheduler.Options{Store: st.Jobs(), Events: bus})
	hub := events.NewHub(bus, time.Hour)
	a, err := app.New(ctx, app.Deps{Config: s.cfg.Load, Store: st, Bus: bus, Health: reg, Jobs: jobs,
		HTTP: client, Plugins: plugin.Default, UIClients: hub.Clients, Log: log,
		Options: app.Options{FastPoll: 50 * time.Millisecond, SlowPoll: 50 * time.Millisecond, MergeWindow: 1500 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	jobs.Start(ctx)
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := api.EnsureAPIKey(ctx, st.Auth()); err != nil {
		t.Fatal(err)
	}
	srv, err := api.New(ctx, api.Deps{Version: "e2e", Started: time.Now(), AuthMode: "password", Auth: st.Auth(), History: st.History(),
		Jobs: jobs, Health: reg, Plugins: plugin.Default, Config: s.cfg.Load, Events: hub, Log: log,
		Mounts: a.Mounts()})
	if err != nil {
		t.Fatal(err)
	}
	s.app = a
	s.api = httptest.NewServer(srv.Handler())
	s.key, _, _ = st.Auth().APIKey(ctx)
	t.Cleanup(func() {
		s.api.Close()
		cancel()
		a.Close()
		_ = jobs.Stop(context.Background())
		bus.Close()
		_ = st.Close()
	})
}

func (s *stack) call(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, s.api.URL+path, strings.NewReader(body))
	req.Header.Set("X-Api-Key", s.key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (s *stack) history(t *testing.T) []string {
	_, body := s.call(t, "GET", "/api/v1/history?limit=200", "")
	var page struct {
		Items []struct {
			Event struct {
				Type string `json:"type"`
			} `json:"event"`
		} `json:"items"`
	}
	_ = json.Unmarshal([]byte(body), &page)
	var out []string
	for _, it := range page.Items {
		out = append(out, it.Event.Type)
	}
	return out
}

func has(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func (s *stack) addMovie(t *testing.T) {
	t.Helper()
	if code, body := s.call(t, "POST", "/api/v1/movies", `{"tmdbId":550,"searchNow":true}`); code != 201 {
		t.Fatalf("add movie: %d %s", code, body)
	}
}

func TestMoviePipelineEndToEnd(t *testing.T) {
	s := start(t, []string{"movie.mkv", "movie.srt"})
	s.addMovie(t)
	testkit.Eventually(t, 20*time.Second, "import.completed in history", func() bool { return has(s.history(t), "import.completed") })

	folder := filepath.Join(s.library, "Example Movie (2021)")
	dest := filepath.Join(folder, "Example Movie (2021) [WEBDL-1080p] - GRP.mkv")
	src, err1 := os.Stat(filepath.Join(s.dl, releaseTitle, "movie.mkv"))
	dst, err2 := os.Stat(dest)
	if err1 != nil || err2 != nil || !os.SameFile(src, dst) {
		t.Fatalf("hardlink into the library: %v %v", err1, err2)
	}
	testkit.Eventually(t, 5*time.Second, "Plex refresh of the movie folder", func() bool { return has(s.plex.refreshed(), folder) })
	testkit.Eventually(t, 5*time.Second, "one ntfy message", func() bool { return len(s.ntfy.messages("/corsarr")) >= 1 })
	time.Sleep(300 * time.Millisecond) // nothing else may arrive
	if msgs := s.ntfy.messages("/corsarr"); len(msgs) != 1 || msgs[0] != "Imported: Example Movie" {
		t.Fatalf("ntfy got %v", msgs)
	}
	// the grab and the import of one download reach a grab+import connection as one message
	// with the connection's own title template
	if msgs := s.ntfy.messages("/both"); len(msgs) != 1 || msgs[0] != "[corsarr] Imported: Example Movie" {
		t.Fatalf("batched: %v", msgs)
	}
	if code, body := s.call(t, "POST", "/api/v1/notifications/both/test", ""); code != 200 || !strings.Contains(body, `"sent":true`) {
		t.Fatalf("test: %d %s", code, body)
	}
	testkit.Eventually(t, 5*time.Second, "test message through the template", func() bool { return len(s.ntfy.messages("/both")) == 2 })
	hist := s.history(t)
	for _, want := range []string{"item.added", "grab.sent", "import.completed"} {
		if !has(hist, want) {
			t.Errorf("history lacks %s: %v", want, hist)
		}
	}
	// the decision and its reasons are visible in interactive search
	code, body := s.call(t, "GET", "/api/v1/movies/1/releases", "")
	if code != 200 || !strings.Contains(body, `"ruleId":"history.grabbed"`) || !strings.Contains(body, `"summary"`) {
		t.Fatalf("interactive search: %d %s", code, body)
	}
	if code, body := s.call(t, "GET", "/api/v1/queue", ""); code != 200 || strings.Contains(body, releaseTitle) {
		t.Fatalf("imported downloads leave the queue: %s", body)
	}
	if _, body := s.call(t, "GET", "/api/v1/movies/1", ""); !strings.Contains(body, `"fileId":1`) {
		t.Fatalf("movie has its file: %s", body)
	}
	if _, body := s.call(t, "GET", "/api/v1/wanted/missing", ""); strings.Contains(body, movieTitle) {
		t.Fatalf("imported movie is no longer missing: %s", body)
	}
}

func TestUnsafePayloadIsRemovedBlocklistedAndResearched(t *testing.T) {
	s := start(t, []string{"movie.mkv", "movie.mkv.scr"})
	s.tz.alt = true
	s.addMovie(t)
	testkit.Eventually(t, 20*time.Second, "download removed with files", s.qb.deletedWithFiles)
	testkit.Eventually(t, 10*time.Second, "blocklist entry", func() bool {
		_, body := s.call(t, "GET", "/api/v1/blocklist", "")
		return strings.Contains(body, releaseTitle) && strings.Contains(body, `"source":"payload"`)
	})
	// the re-search runs after the removal, so it grabs the clean alternative
	// instead of treating the rejected download as queued
	testkit.Eventually(t, 10*time.Second, "re-search grabs the alternative", func() bool { return s.qb.added(altHash) })
	testkit.Eventually(t, 20*time.Second, "alternative imported", func() bool { return has(s.history(t), "import.completed") })
	hist := s.history(t)
	for _, want := range []string{"grab.sent", "payload.rejected", "download.removed", "blocklist.added"} {
		if !has(hist, want) {
			t.Errorf("history lacks %s: %v", want, hist)
		}
	}
	if s.qb.added(infoHash) {
		t.Fatal("the rejected release was grabbed again")
	}
	_ = filepath.WalkDir(s.library, func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(p, ".scr") {
			t.Errorf("unsafe file reached the library: %s", p)
		}
		return nil
	})
	for _, m := range s.ntfy.messages("/corsarr") {
		if m != "Imported: Example Movie" {
			t.Fatalf("import-only connection sent %q", m)
		}
	}
}

func TestShadowModeDecidesButChangesNothing(t *testing.T) {
	s := startIn(t, domain.ModeShadow, []string{"movie.mkv"})
	s.addMovie(t)
	testkit.Eventually(t, 10*time.Second, "decision.shadowed in history", func() bool { return has(s.history(t), "decision.shadowed") })
	time.Sleep(300 * time.Millisecond)
	if s.qb.added(infoHash) {
		t.Fatal("shadow mode sent a grab to the downloader")
	}
	for _, never := range []string{"grab.sent", "import.completed"} {
		if has(s.history(t), never) {
			t.Fatalf("shadow mode produced %s", never)
		}
	}
	if n := len(s.ntfy.messages("/corsarr")); n != 0 {
		t.Fatalf("shadow mode notified %d times", n)
	}
	// the queued search on add recorded its state, shown on Wanted
	if _, body := s.call(t, "GET", "/api/v1/wanted/missing", ""); !strings.Contains(body, `"lastSearched"`) || !strings.Contains(body, `"nextEligible"`) {
		t.Fatalf("wanted lacks the search state: %s", body)
	}
	// a manual grab is shadowed as well
	if code, body := s.call(t, "POST", "/api/v1/movies/1/search", ""); code != 200 || !strings.Contains(body, `"shadowed":"`+releaseTitle) {
		t.Fatalf("search in shadow: %d %s", code, body)
	}
	// switching to active applies without a restart
	s.setMode(t, domain.ModeShadow, domain.ModeActive)
	testkit.Eventually(t, 5*time.Second, "mediakind.mode_changed", func() bool { return has(s.history(t), "mediakind.mode_changed") })
	if code, body := s.call(t, "POST", "/api/v1/movies/1/search", ""); code != 200 || !strings.Contains(body, `"grabbed":"`+releaseTitle) {
		t.Fatalf("search after activation: %d %s", code, body)
	}
	testkit.Eventually(t, 20*time.Second, "import after activation", func() bool { return has(s.history(t), "import.completed") })
}

func TestDisabledModeRefusesMovieRoutes(t *testing.T) {
	s := startIn(t, domain.ModeDisabled, []string{"movie.mkv"})
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/v1/movies", ""},
		{"POST", "/api/v1/movies", `{"tmdbId":550,"searchNow":true}`},
		{"POST", "/api/v1/movies/1/search", ""},
		{"GET", "/api/v1/movies/1/releases", ""},
	} {
		code, body := s.call(t, c.method, c.path, c.body)
		if c.path == "/api/v1/movies/1/search" || c.path == "/api/v1/movies/1/releases" {
			if code != 409 && code != 404 {
				t.Errorf("%s %s: %d %s", c.method, c.path, code, body)
			}
			continue
		}
		if code != 409 || !strings.Contains(body, "kind_mode") {
			t.Errorf("%s %s: %d %s", c.method, c.path, code, body)
		}
	}
	if s.tz.count() != 0 {
		t.Fatal("disabled kind searched an indexer")
	}
}

func TestMetadataOutageServesTheCache(t *testing.T) {
	s := start(t, []string{"movie.mkv"})
	s.addMovie(t)
	s.tmdb.Close() // the provider goes down
	for _, path := range []string{"/api/v1/movies/lookup?term=tmdb:550", "/api/v1/movies/lookup?term=example", "/api/v1/movies/1"} {
		if code, body := s.call(t, "GET", path, ""); code != 200 || !strings.Contains(body, movieTitle) {
			t.Errorf("%s while the provider is down: %d %s", path, code, body)
		}
	}
}

func TestMovingAMovieToAnotherRootFolder(t *testing.T) {
	s := start(t, []string{"movie.mkv"})
	s.addMovie(t)
	testkit.Eventually(t, 20*time.Second, "import.completed in history", func() bool { return has(s.history(t), "import.completed") })
	folder := filepath.Join(s.shelf, "Example Movie (2021)")
	if code, body := s.call(t, "GET", "/api/v1/movies/1/move?rootFolder="+url.QueryEscape(s.shelf), ""); code != 200 || !strings.Contains(body, folder) {
		t.Fatalf("preview: %d %s", code, body)
	}
	code, body := s.call(t, "PUT", "/api/v1/movies/1", `{"rootFolder":"`+s.shelf+`"}`)
	if code != 200 || !strings.Contains(body, `"path":"`+folder+`"`) {
		t.Fatalf("move: %d %s", code, body)
	}
	if _, err := os.Stat(filepath.Join(folder, "Example Movie (2021) [WEBDL-1080p] - GRP.mkv")); err != nil {
		t.Fatal("file moved with the folder:", err)
	}
	if _, err := os.Stat(filepath.Join(s.library, "Example Movie (2021)")); !os.IsNotExist(err) {
		t.Fatal("old folder left behind")
	}
	testkit.Eventually(t, 5*time.Second, "Plex refresh of the new folder", func() bool { return has(s.plex.refreshed(), folder) })
	if !has(s.history(t), "file.renamed") {
		t.Fatal("the move is in History")
	}
}
