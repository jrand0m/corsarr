package legacy_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/indexers"
	"github.com/jrand0m/corsarr/internal/legacy"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/musicauto"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/testkit"
	"github.com/jrand0m/corsarr/internal/tv"
	"github.com/jrand0m/corsarr/internal/tvauto"
)

var (
	day0  = time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	ctx   = context.Background()
	aired = day0.Add(-30 * 24 * time.Hour)
	later = day0.Add(24 * time.Hour)
)

// ── fakes ────────────────────────────────────────────────────────────────────

// meta is the TV and movie metadata provider.
type meta struct{ down bool }

func (meta) Test(context.Context) error { return nil }

func (m meta) SearchSeries(_ context.Context, q string) ([]plugin.SeriesRecord, error) {
	if m.down {
		return nil, errors.New("provider down")
	}
	rec := showRecord()
	rec.Episodes = nil
	return []plugin.SeriesRecord{rec}, nil
}

func (meta) Series(_ context.Context, ids map[string]string) (plugin.SeriesRecord, error) {
	if ids["tmdb"] == "77" || ids["tvdb"] == "321" {
		return showRecord(), nil
	}
	return plugin.SeriesRecord{}, plugin.ErrNotFound
}

func showRecord() plugin.SeriesRecord {
	return plugin.SeriesRecord{IDs: map[string]string{"tmdb": "77", "tvdb": "321", "imdb": "tt77"}, Title: "Example Show", Year: 2020,
		Status: "continuing", Network: "Net", PosterURL: "https://img.invalid/p.jpg", Genres: []string{"Drama"}, SeasonCount: 2,
		Episodes: []plugin.EpisodeRecord{{Season: 1, Number: 1, Title: "Pilot", AirAt: &aired}, {Season: 1, Number: 2, Title: "Two", AirAt: &aired},
			{Season: 2, Number: 1, Title: "Next", AirAt: &later}}}
}

func (meta) SearchMovies(context.Context, string, int) ([]plugin.MovieRecord, error) {
	return []plugin.MovieRecord{movieRecord(), {IDs: map[string]string{"tmdb": "551"}, Title: "Other"}}, nil
}

func (meta) Movie(_ context.Context, ids map[string]string) (plugin.MovieRecord, error) {
	if ids["tmdb"] == "550" {
		return movieRecord(), nil
	}
	return plugin.MovieRecord{}, plugin.ErrNotFound
}

func movieRecord() plugin.MovieRecord {
	cinema, digital := aired.Add(-60*24*time.Hour), aired
	return plugin.MovieRecord{IDs: map[string]string{"tmdb": "550", "imdb": "tt550"}, Title: "Example Movie", Year: 2024,
		RuntimeMin: 100, InCinemas: &cinema, Digital: &digital, PosterURL: "https://img.invalid/m.jpg"}
}

// searches records the TV searches it is asked for.
type searches struct {
	mu    sync.Mutex
	calls []string
	index *tvauto.Index
	fail  bool
}

func (s *searches) record(call string) (tvauto.Summary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
	if s.fail {
		return tvauto.Summary{}, errors.New("indexers down")
	}
	return tvauto.Summary{}, nil
}

func (s *searches) SearchSeries(_ context.Context, id int64, _ domain.SearchOrigin) (tvauto.Summary, error) {
	return s.record(fmt.Sprintf("series %d", id))
}

func (s *searches) SearchSeason(_ context.Context, id int64, n int, _ domain.SearchOrigin) (tvauto.Summary, error) {
	return s.record(fmt.Sprintf("season %d/%d", id, n))
}

func (s *searches) SearchEpisode(_ context.Context, id int64, _ domain.SearchOrigin) (tvauto.Summary, error) {
	return s.record(fmt.Sprintf("episode %d", id))
}

func (s *searches) Index() *tvauto.Index { return s.index }

func (s *searches) got() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

type movieSearch struct {
	mu  sync.Mutex
	ids []int64
}

func (m *movieSearch) Request(_ context.Context, id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ids = append(m.ids, id)
}

type artists []domain.Artist

func (a artists) EachArtist(_ context.Context, fn func(domain.Artist) error) error {
	for _, x := range a {
		if err := fn(x); err != nil {
			return err
		}
	}
	return nil
}

type musicWanted struct{ index *musicauto.Index }

func (m musicWanted) Index() *musicauto.Index { return m.index }

type roots []domain.RootFolder

func (r roots) List(context.Context) ([]domain.RootFolder, error) { return r, nil }

type indexerList []indexers.Row

func (l indexerList) List(context.Context) ([]indexers.Row, error) { return l, nil }

// ── harness ──────────────────────────────────────────────────────────────────

type env struct {
	t       *testing.T
	st      *store.Store
	srv     *httptest.Server
	key     string
	base    string
	cfg     atomic.Pointer[config.Config]
	mode    atomic.Value // domain.KindMode for every kind
	search  *searches
	movies  *movieSearch
	tvDir   string
	movDir  string
	TV      *tv.Service
	Movies  *movies.Service
	running sync.WaitGroup
}

func newEnv(t *testing.T, basePath string, apps ...string) *env {
	t.Helper()
	e := &env{t: t, st: testkit.Store(t), base: strings.TrimSuffix(basePath, "/"), tvDir: t.TempDir(), movDir: t.TempDir(),
		search: &searches{index: tvauto.NewIndex()}, movies: &movieSearch{}}
	e.mode.Store(domain.ModeActive)
	e.setApps(apps...)
	modes := domain.Modes(func(domain.MediaKind) domain.KindMode { return e.mode.Load().(domain.KindMode) })
	now := func() time.Time { return day0 }
	pub := &testkit.Recorder{}
	e.TV = tv.New(tv.Deps{Repo: e.st.TV(), Files: e.st.Library(), Pub: pub, Metadata: func() (plugin.SeriesMetadata, bool) { return meta{}, true },
		RootFolders: func() []string { return []string{e.tvDir} }, Modes: modes, Now: now})
	e.Movies = movies.New(movies.Deps{Repo: e.st.Movies(), Pub: pub, Metadata: func() (plugin.MovieMetadata, bool) { return meta{}, true },
		RootFolders: func() []string { return []string{e.movDir} }, Modes: modes, Now: now})
	mw := musicauto.NewIndex()
	release := aired
	mw.PutAlbum(musicauto.Entry{Album: domain.Album{ID: 5, ArtistID: 3, MBID: "al-mbid", Title: "First", ReleaseDate: &release, Monitored: true},
		Artist: music.ArtistRef{ID: 3, Name: "Band"}, Reason: automation.ReasonMissing})
	svc := legacy.New(legacy.Deps{Config: e.cfg.Load, Modes: modes, TV: e.TV, TVSearch: e.search, Movies: e.Movies, MovieSearch: e.movies,
		Music:       artists{{ID: 3, MBID: "ar-mbid", Name: "Band", SortName: "band", Monitored: true, ProfileKey: "lossless", Tags: []string{"loud"}, AddedAt: day0}},
		MusicWanted: musicWanted{mw}, Queue: e.st.Queue(),
		RootFolders: roots{{Path: e.tvDir, Kind: domain.KindTV, Accessible: true, FreeSpace: 1 << 30}, {Path: e.movDir, Kind: domain.KindMovie, Accessible: true}},
		Indexers:    indexerList{{Key: "fast", Name: "Fast", Enabled: true, Priority: 25, Plugin: "torznab", PluginName: "Torznab"}, {Key: "slow", Enabled: false}},
		Queries:     e.st.Searches(), Grabs: e.st.Queue(), IDs: e.st.LegacyIDs(),
		Go: func(fn func(context.Context)) {
			e.running.Add(1)
			go func() { defer e.running.Done(); fn(ctx) }()
		},
		Now: now})
	if err := api.EnsureAPIKey(ctx, e.st.Auth()); err != nil {
		t.Fatal(err)
	}
	srv, err := api.New(ctx, api.Deps{BasePath: basePath, AuthMode: "password", Auth: e.st.Auth(), Config: e.cfg.Load,
		Mounts: []func(api.Mounter){svc.Mount, func(m api.Mounter) {}}})
	if err != nil {
		t.Fatal(err)
	}
	e.key, _, _ = e.st.Auth().APIKey(ctx)
	e.srv = httptest.NewServer(srv.Handler())
	t.Cleanup(e.srv.Close)
	t.Cleanup(e.running.Wait)
	return e
}

func (e *env) setApps(apps ...string) {
	c := &config.Config{Legacy: map[string]bool{}}
	c.Server.BasePath = e.base
	for _, a := range apps {
		c.Legacy[a] = true
	}
	e.cfg.Store(c)
}

// do sends a request with the API key and returns status and body.
func (e *env) do(method, path, body string, hdr ...string) (int, string) {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+e.base+path, strings.NewReader(body))
	req.Header.Set("X-Api-Key", e.key)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		e.t.Fatalf("%s %s: content type %q", method, path, ct)
	}
	return resp.StatusCode, string(b)
}

// json calls a route, checks the status and decodes the body.
func (e *env) json(method, path, body string, want int, v any) {
	e.t.Helper()
	code, out := e.do(method, path, body)
	if code != want {
		e.t.Fatalf("%s %s: %d %s", method, path, code, out)
	}
	if v != nil {
		if err := json.Unmarshal([]byte(out), v); err != nil {
			e.t.Fatalf("%s %s: %v in %s", method, path, err, out)
		}
	}
}

// fields decodes a JSON object or the first element of an array into a
// generic map so field names and JSON types can be checked.
func fields(t *testing.T, body string) map[string]any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("%v in %s", err, body)
	}
	if list, isList := v.([]any); isList {
		if len(list) == 0 {
			t.Fatalf("empty list")
		}
		v = list[0]
	}
	return v.(map[string]any)
}

// hasTypes checks each named field's JSON type: n number, s string, b bool, a array, o object.
func hasTypes(t *testing.T, obj map[string]any, spec string) {
	t.Helper()
	for _, f := range strings.Fields(spec) {
		name, kind, _ := strings.Cut(f, ":")
		v, present := obj[name]
		if !present {
			t.Errorf("field %s missing in %v", name, obj)
			continue
		}
		var ok bool
		switch kind {
		case "n":
			_, ok = v.(float64)
		case "s":
			_, ok = v.(string)
		case "b":
			_, ok = v.(bool)
		case "a":
			_, ok = v.([]any)
		case "o":
			_, ok = v.(map[string]any)
		}
		if !ok {
			t.Errorf("field %s: %T, want %s", name, v, kind)
		}
	}
}

// grab puts a download of kind with items into the queue.
func (e *env) grab(kind domain.MediaKind, hash string, state domain.DownloadState, refs ...domain.ItemRef) store.StoredDownload {
	e.t.Helper()
	links := make([]domain.DownloadLink, len(refs))
	for i, r := range refs {
		links[i] = domain.DownloadLink{Item: r, Status: domain.LinkPending}
	}
	g := domain.Grab{Kind: kind, ItemRefs: refs, Origin: domain.OriginAutomatic, Downloader: "qb", DownloadID: hash, SentAt: day0,
		Release:  domain.Release{ID: hash, Title: "Release." + hash, InfoHash: hash, Indexers: []domain.IndexerRef{{Key: "fast"}}},
		Decision: domain.Decision{Accepted: true}}
	_, sd, err := e.st.Queue().CreateGrab(ctx, g, domain.Download{Downloader: "qb", DownloadID: hash, Protocol: domain.ProtoTorrent,
		Kind: kind, Title: g.Release.Title, State: domain.StateQueued, Size: 1000, AddedAt: day0, Links: links})
	if err != nil {
		e.t.Fatal(err)
	}
	sd.State, sd.Downloaded, sd.ETASeconds = state, 400, 90061
	if err := e.st.Queue().Save(ctx, sd); err != nil {
		e.t.Fatal(err)
	}
	return sd
}

func (e *env) waitBackground() { e.running.Wait() }

// ── gating and auth ──────────────────────────────────────────────────────────

func TestEveryAppIsOffByDefault(t *testing.T) {
	e := newEnv(t, "")
	for _, p := range []string{"/legacy/sonarr/api/v3/system/status", "/legacy/radarr/api/v3/system/status",
		"/legacy/lidarr/api/v1/system/status", "/legacy/prowlarr/api/v1/system/status", "/legacy/readarr/api/v1/system/status"} {
		if code, body := e.do("GET", p, ""); code != 404 || !strings.Contains(body, `"not_found"`) {
			t.Errorf("%s: %d %s", p, code, body)
		}
		if code, _ := e.do("GET", p, "", "X-Api-Key", "wrong"); code != 404 {
			t.Errorf("%s without a valid key: %d", p, code)
		}
	}
	e.setApps(config.LegacyRadarr) // live switch: only radarr answers
	if code, _ := e.do("GET", "/legacy/radarr/api/v3/system/status", ""); code != 200 {
		t.Fatalf("radarr on: %d", code)
	}
	if code, _ := e.do("GET", "/legacy/sonarr/api/v3/system/status", ""); code != 404 {
		t.Fatalf("sonarr still off: %d", code)
	}
}

func TestAPIKeyAuth(t *testing.T) {
	e := newEnv(t, "", config.LegacySonarr)
	const p = "/legacy/sonarr/api/v3/system/status"
	if code, body := e.do("GET", p, "", "X-Api-Key", "wrong"); code != 401 || !strings.Contains(body, "message") {
		t.Fatalf("wrong key: %d %s", code, body)
	}
	resp, err := http.Get(e.srv.URL + p + "?apikey=" + e.key)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("apikey query: %d", resp.StatusCode)
	}
	resp, _ = http.Get(e.srv.URL + p)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("no key: %d", resp.StatusCode)
	}
}

func TestUnknownRoutesAnswer404JSON(t *testing.T) {
	e := newEnv(t, "", config.LegacySonarr, config.LegacyRadarr, config.LegacyLidarr, config.LegacyProwlarr)
	for _, c := range [][2]string{{"GET", "/legacy/sonarr/api/v3/episodefile"}, {"DELETE", "/legacy/sonarr/api/v3/series/1"},
		{"POST", "/legacy/lidarr/api/v1/artist"}, {"GET", "/legacy/prowlarr/api/v1/search"}, {"GET", "/legacy/radarr/api/v3/movie/abc"},
		{"GET", "/legacy/sonarr/api/v3/command/99"}} {
		if code, body := e.do(c[0], c[1], ""); code != 404 || !strings.Contains(body, "message") {
			t.Errorf("%s %s: %d %s", c[0], c[1], code, body)
		}
	}
}

func TestStatusHonoursTheBasePath(t *testing.T) {
	e := newEnv(t, "/corsarr", config.LegacySonarr, config.LegacyRadarr, config.LegacyLidarr, config.LegacyProwlarr)
	for app, want := range map[string]string{"sonarr/api/v3": "4.", "radarr/api/v3": "5.", "lidarr/api/v1": "2.", "prowlarr/api/v1": "1."} {
		code, body := e.do("GET", "/legacy/"+app+"/system/status", "")
		obj := fields(t, body)
		hasTypes(t, obj, "appName:s version:s urlBase:s startTime:s")
		if code != 200 || !strings.HasPrefix(obj["version"].(string), want) || !strings.HasPrefix(obj["urlBase"].(string), "/corsarr/legacy/") {
			t.Errorf("%s: %d %s", app, code, body)
		}
	}
}

// ── Sonarr ───────────────────────────────────────────────────────────────────

const sonarr = "/legacy/sonarr/api/v3"

func (e *env) profileID(base, name string) int64 {
	var list []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	e.json("GET", base+"/qualityprofile", "", 200, &list)
	for _, p := range list {
		if p.Name == name {
			return p.ID
		}
	}
	e.t.Fatalf("no profile %s in %+v", name, list)
	return 0
}

func TestSonarrListsProfilesRootFoldersAndTags(t *testing.T) {
	e := newEnv(t, "", config.LegacySonarr)
	_, body := e.do("GET", sonarr+"/qualityprofile", "")
	hasTypes(t, fields(t, body), "id:n name:s")
	id := e.profileID(sonarr, "HD-1080p")
	if again := e.profileID(sonarr, "HD-1080p"); again != id || id == 0 {
		t.Fatalf("profile ids are stable: %d %d", id, again)
	}
	var rootList []map[string]any
	e.json("GET", sonarr+"/rootfolder", "", 200, &rootList)
	if len(rootList) != 1 || rootList[0]["path"] != e.tvDir {
		t.Fatalf("only TV root folders: %v", rootList)
	}
	hasTypes(t, rootList[0], "id:n path:s freeSpace:n accessible:b unmappedFolders:a")
	if code, body := e.do("GET", sonarr+"/languageprofile", ""); code != 200 || strings.TrimSpace(body) != "[]" {
		t.Fatalf("language profiles: %d %s", code, body)
	}
	var tg struct {
		ID    int64  `json:"id"`
		Label string `json:"label"`
	}
	e.json("POST", sonarr+"/tag", `{"label":" 2 - bob "}`, 201, &tg)
	if tg.ID == 0 || tg.Label != "2 - bob" {
		t.Fatalf("tag %+v", tg)
	}
	if code, _ := e.do("POST", sonarr+"/tag", `{"label":""}`, ""); code != 400 {
		t.Fatalf("empty label: %d", code)
	}
	if code, _ := e.do("POST", sonarr+"/tag", `{`, ""); code != 400 {
		t.Fatalf("bad JSON: %d", code)
	}
	// a tag set in the UI shows up too
	if _, _, err := e.TV.Add(ctx, tv.AddRequest{TMDBID: 77, Tags: []string{"kids"}}); err != nil {
		t.Fatal(err)
	}
	var tags []struct {
		Label string `json:"label"`
	}
	e.json("GET", sonarr+"/tag", "", 200, &tags)
	if len(tags) != 2 || tags[0].Label != "2 - bob" || tags[1].Label != "kids" {
		t.Fatalf("tags %+v", tags)
	}
}

type seriesOut struct {
	ID               int64   `json:"id"`
	TvdbID           int64   `json:"tvdbId"`
	TmdbID           int64   `json:"tmdbId"`
	Monitored        bool    `json:"monitored"`
	QualityProfileID int64   `json:"qualityProfileId"`
	SeriesType       string  `json:"seriesType"`
	SeasonFolder     bool    `json:"seasonFolder"`
	Tags             []int64 `json:"tags"`
	Seasons          []struct {
		SeasonNumber int  `json:"seasonNumber"`
		Monitored    bool `json:"monitored"`
		Statistics   struct {
			TotalEpisodeCount int `json:"totalEpisodeCount"`
			EpisodeCount      int `json:"episodeCount"`
		} `json:"statistics"`
	} `json:"seasons"`
}

func TestSonarrLookupAddAndEdit(t *testing.T) {
	e := newEnv(t, "", config.LegacySonarr)
	code, body := e.do("GET", sonarr+"/series/lookup?term=tvdb:321", "")
	obj := fields(t, body)
	hasTypes(t, obj, "id:n title:s tvdbId:n tmdbId:n year:n seasons:a images:a tags:a titleSlug:s monitored:b status:s")
	if code != 200 || obj["id"].(float64) != 0 || obj["tvdbId"].(float64) != 321 || len(obj["seasons"].([]any)) != 2 {
		t.Fatalf("lookup: %d %s", code, body)
	}
	if code, body := e.do("GET", sonarr+"/series/lookup?term=example", ""); code != 200 || !strings.Contains(body, `"tvdbId":321`) {
		t.Fatalf("text lookup: %d %s", code, body)
	}
	var tg struct {
		ID int64 `json:"id"`
	}
	e.json("POST", sonarr+"/tag", `{"label":"1 - alice"}`, 201, &tg)
	hd := e.profileID(sonarr, "HD-1080p")
	add := fmt.Sprintf(`{"tvdbId":321,"title":"Example Show","qualityProfileId":%d,"languageProfileId":1,"rootFolderPath":%q,
		"seasons":[{"seasonNumber":1,"monitored":false},{"seasonNumber":2,"monitored":true}],"tags":[%d,999],"seasonFolder":true,
		"monitored":true,"seriesType":"standard","addOptions":{"ignoreEpisodesWithFiles":true,"searchForMissingEpisodes":false}}`, hd, e.tvDir, tg.ID)
	var s seriesOut
	e.json("POST", sonarr+"/series", add, 201, &s)
	if s.ID == 0 || s.TmdbID != 77 || s.QualityProfileID != hd || len(s.Tags) != 1 || s.Tags[0] != tg.ID ||
		len(s.Seasons) != 2 || s.Seasons[0].Monitored || !s.Seasons[1].Monitored || s.Seasons[0].Statistics.TotalEpisodeCount != 2 {
		t.Fatalf("added %+v", s)
	}
	_, body = e.do("GET", fmt.Sprintf("%s/series/%d", sonarr, s.ID), "")
	hasTypes(t, fields(t, body), "id:n path:s rootFolderPath:s qualityProfileId:n languageProfileId:n seasonFolder:b statistics:o added:s")
	var again seriesOut
	e.json("POST", sonarr+"/series", add, 200, &again) // already in the library
	if again.ID != s.ID {
		t.Fatalf("re-add %+v", again)
	}
	var found []seriesOut
	e.json("GET", sonarr+"/series/lookup?term=tvdb:321", "", 200, &found)
	if len(found) != 1 || found[0].ID != s.ID {
		t.Fatalf("lookup marks the library series: %+v", found)
	}

	sd := e.profileID(sonarr, "Any")
	edit := fmt.Sprintf(`{"id":%d,"tvdbId":321,"monitored":false,"qualityProfileId":%d,"seriesType":"anime","seasonFolder":false,"tags":[],
		"seasons":[{"seasonNumber":1,"monitored":true},{"seasonNumber":2,"monitored":true},{"seasonNumber":9,"monitored":true}]}`, s.ID, sd)
	var edited seriesOut
	e.json("PUT", sonarr+"/series", edit, 202, &edited)
	if edited.Monitored || edited.QualityProfileID != sd || edited.SeriesType != "anime" || edited.SeasonFolder || len(edited.Tags) != 0 ||
		!edited.Seasons[0].Monitored || !edited.Seasons[1].Monitored {
		t.Fatalf("edited %+v", edited)
	}
	e.json("PUT", fmt.Sprintf("%s/series/%d", sonarr, s.ID), `{"monitored":true,"seasons":[{"seasonNumber":1,"monitored":false}]}`, 202, &edited)
	if !edited.Monitored || edited.Seasons[0].Monitored {
		t.Fatalf("edited by path %+v", edited)
	}
	var list []seriesOut
	e.json("GET", sonarr+"/series?tvdbId=321", "", 200, &list)
	if len(list) != 1 {
		t.Fatalf("filter %+v", list)
	}
	e.json("GET", sonarr+"/series?tvdbId=1", "", 200, &list)
	if len(list) != 0 {
		t.Fatalf("filter miss %+v", list)
	}
}

func TestSonarrAddOptionsAndErrors(t *testing.T) {
	e := newEnv(t, "", config.LegacySonarr)
	var s seriesOut
	e.json("POST", sonarr+"/series", `{"tmdbId":77,"monitored":false,"addOptions":{"monitor":"pilot","searchForMissingEpisodes":true}}`, 201, &s)
	if s.Monitored || s.Seasons[0].Monitored {
		t.Fatalf("pilot, unmonitored: %+v", s)
	}
	for body, want := range map[string]int{
		`{"title":"x"}`:                           400, // no id
		`{"tvdbId":999}`:                          400, // provider has no such series
		`{"tvdbId":321,"qualityProfileId":999}`:   400,
		`{"tvdbId":321,"rootFolderPath":"/nope"}`: 400,
		`{"tvdbId":321,"seriesType":"weird"}`:     400,
		`[`:                                       400,
	} {
		if code, out := e.do("POST", sonarr+"/series", body); code != want {
			t.Errorf("%s: %d %s", body, code, out)
		}
	}
	for path, want := range map[string]int{"/series/0": 404, "/series/42": 404, "/series/x": 404, "/episode": 400, "/episode?seriesId=42": 404} {
		if code, out := e.do("GET", sonarr+path, ""); code != want {
			t.Errorf("%s: %d %s", path, code, out)
		}
	}
	for path, body := range map[string]string{"/series/42": `{}`, "/series": `{"id":42}`} {
		if code, _ := e.do("PUT", sonarr+path, body); code != 404 {
			t.Errorf("PUT %s: %d", path, code)
		}
	}
	if code, _ := e.do("PUT", fmt.Sprintf("%s/series/%d", sonarr, s.ID), `{"qualityProfileId":999}`); code != 400 {
		t.Errorf("unknown profile on edit: %d", code)
	}
	if code, _ := e.do("PUT", sonarr+"/series/0", `{}`); code != 404 {
		t.Errorf("zero id: %d", code)
	}
	e.mode.Store(domain.ModeDisabled)
	for _, c := range [][3]string{{"POST", "/series", `{"tvdbId":321}`}, {"PUT", "/series/1", `{}`}, {"POST", "/command", `{"name":"SeriesSearch","seriesId":1}`}} {
		if code, _ := e.do(c[0], sonarr+c[1], c[2]); code != 409 {
			t.Errorf("disabled TV %s %s: %d", c[0], c[1], code)
		}
	}
}

func TestSonarrEpisodesCommandsQueueWantedAndCalendar(t *testing.T) {
	e := newEnv(t, "", config.LegacySonarr)
	ser, _, err := e.TV.Add(ctx, tv.AddRequest{TMDBID: 77})
	if err != nil {
		t.Fatal(err)
	}
	var eps []map[string]any
	e.json("GET", fmt.Sprintf("%s/episode?seriesId=%d", sonarr, ser.ID), "", 200, &eps)
	if len(eps) != 3 {
		t.Fatalf("episodes %v", eps)
	}
	hasTypes(t, eps[0], "id:n seriesId:n seasonNumber:n episodeNumber:n title:s airDate:s airDateUtc:s hasFile:b monitored:b episodeFileId:n")
	ep1, ep2 := int64(eps[0]["id"].(float64)), int64(eps[1]["id"].(float64))

	for _, body := range []string{fmt.Sprintf(`{"name":"SeriesSearch","seriesId":%d}`, ser.ID),
		fmt.Sprintf(`{"name":"SeasonSearch","seriesId":%d,"seasonNumber":1}`, ser.ID),
		fmt.Sprintf(`{"name":"EpisodeSearch","episodeIds":[%d,%d]}`, ep1, ep2)} {
		code, out := e.do("POST", sonarr+"/command", body)
		obj := fields(t, out)
		hasTypes(t, obj, "id:n name:s status:s queued:s body:o")
		if code != 201 {
			t.Fatalf("%s: %d %s", body, code, out)
		}
	}
	e.waitBackground()
	want := []string{fmt.Sprintf("series %d", ser.ID), fmt.Sprintf("season %d/1", ser.ID), fmt.Sprintf("episode %d", ep1), fmt.Sprintf("episode %d", ep2)}
	if got := e.search.got(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("searches %v, want %v", got, want)
	}
	var cmd struct {
		Status string `json:"status"`
	}
	e.json("GET", sonarr+"/command/1", "", 200, &cmd)
	if cmd.Status != "completed" {
		t.Fatalf("command 1: %+v", cmd)
	}
	e.search.fail = true
	e.json("POST", sonarr+"/command", fmt.Sprintf(`{"name":"SeriesSearch","seriesId":%d}`, ser.ID), 201, nil)
	e.waitBackground()
	e.json("GET", sonarr+"/command/4", "", 200, &cmd)
	if cmd.Status != "failed" {
		t.Fatalf("failed search: %+v", cmd)
	}
	for _, body := range []string{`{"name":"RssSync"}`, `{"name":"SeriesSearch"}`, `{"name":"SeasonSearch","seriesId":1}`, `{"name":"EpisodeSearch"}`} {
		if code, _ := e.do("POST", sonarr+"/command", body); code != 400 {
			t.Errorf("%s: %d", body, code)
		}
	}

	e.grab(domain.KindTV, "aaa", domain.StateDownloading, domain.ItemRef{Kind: domain.KindTV, ID: ep1}, domain.ItemRef{Kind: domain.KindTV, ID: ep2})
	held := e.grab(domain.KindTV, "bbb", domain.StateStalled, domain.ItemRef{Kind: domain.KindTV, ID: 999})
	held.Held, held.StatusReason = true, "no seeders"
	if err := e.st.Queue().Save(ctx, held); err != nil {
		t.Fatal(err)
	}
	e.grab(domain.KindMovie, "ccc", domain.StateDownloading, domain.ItemRef{Kind: domain.KindMovie, ID: 1}) // not TV
	code, body := e.do("GET", sonarr+"/queue?page=1&pageSize=2&includeEpisode=true&includeSeries=true", "")
	page := fields(t, body)
	hasTypes(t, page, "page:n pageSize:n totalRecords:n records:a sortKey:s")
	if code != 200 || page["totalRecords"].(float64) != 3 || len(page["records"].([]any)) != 2 {
		t.Fatalf("queue: %d %s", code, body)
	}
	var details []map[string]any
	e.json("GET", sonarr+"/queue/details", "", 200, &details)
	byEpisode := map[float64]map[string]any{}
	for _, d := range details {
		ep, _ := d["episodeId"].(float64)
		byEpisode[ep] = d
	}
	rec := byEpisode[float64(ep2)]
	hasTypes(t, rec, "id:n seriesId:n episodeId:n seasonNumber:n title:s size:n sizeleft:n status:s timeleft:s estimatedCompletionTime:s "+
		"downloadId:s protocol:s trackedDownloadState:s trackedDownloadStatus:s episode:o series:o statusMessages:a")
	if rec["sizeleft"].(float64) != 600 || rec["timeleft"] != "1.01:01:01" || rec["status"] != "downloading" || rec["downloadId"] != "aaa" {
		t.Fatalf("record %v", rec)
	}
	if orphan := byEpisode[0]; orphan["status"] != "paused" || orphan["trackedDownloadStatus"] != "warning" || orphan["series"] != nil {
		t.Fatalf("held, unknown episode %v", orphan)
	}
	var st map[string]any
	e.json("GET", sonarr+"/queue/status", "", 200, &st)
	if st["totalCount"].(float64) != 2 || st["warnings"] != true {
		t.Fatalf("queue status %v", st)
	}

	e.search.index.PutEpisode(tvauto.Entry{Episode: tv.EpisodeView{Episode: domain.Episode{ID: ep1, SeriesID: ser.ID, Season: 1, Number: 1, AirAt: &aired}},
		Series: tv.RefOf(ser), Reason: automation.ReasonMissing})
	_, body = e.do("GET", sonarr+"/wanted/missing?page=1&pageSize=10&sortKey=airDateUtc", "")
	if page := fields(t, body); page["totalRecords"].(float64) != 1 {
		t.Fatalf("wanted: %s", body)
	}
	var cal []map[string]any
	e.json("GET", sonarr+"/calendar?start="+aired.Add(-time.Hour).Format(time.RFC3339)+"&end="+day0.Format("2006-01-02"), "", 200, &cal)
	if len(cal) != 2 || cal[0]["series"].(map[string]any)["title"] != "Example Show" {
		t.Fatalf("calendar %v", cal)
	}
	e.json("GET", sonarr+"/calendar", "", 200, &cal) // default window: today and two days on
	if len(cal) != 1 || cal[0]["title"] != "Next" {
		t.Fatalf("default calendar %v", cal)
	}
}

// ── Radarr ───────────────────────────────────────────────────────────────────

const radarr = "/legacy/radarr/api/v3"

func TestRadarrLookupAddEditCommandAndQueue(t *testing.T) {
	e := newEnv(t, "", config.LegacyRadarr)
	var rootList []map[string]any
	e.json("GET", radarr+"/rootfolder", "", 200, &rootList)
	if len(rootList) != 1 || rootList[0]["path"] != e.movDir {
		t.Fatalf("movie root folders %v", rootList)
	}
	code, body := e.do("GET", radarr+"/movie/lookup?term=tmdb:550", "")
	obj := fields(t, body)
	hasTypes(t, obj, "id:n title:s tmdbId:n year:n hasFile:b monitored:b images:a status:s titleSlug:s minimumAvailability:s")
	if code != 200 || obj["id"].(float64) != 0 || obj["status"] != "released" {
		t.Fatalf("lookup %d %s", code, body)
	}
	if code, body := e.do("GET", radarr+"/movie/lookup?term=example", ""); code != 200 || !strings.Contains(body, `"status":"tba"`) {
		t.Fatalf("text lookup %d %s", code, body)
	}
	hd := e.profileID(radarr, "HD-1080p")
	add := fmt.Sprintf(`{"title":"Example Movie","qualityProfileId":%d,"profileId":%d,"minimumAvailability":"tba","tmdbId":550,"year":2024,
		"rootFolderPath":%q,"monitored":true,"tags":[1],"addOptions":{"searchForMovie":true}}`, hd, hd, e.movDir)
	var m map[string]any
	e.json("POST", radarr+"/movie", add, 201, &m)
	hasTypes(t, m, "id:n path:s rootFolderPath:s qualityProfileId:n isAvailable:b hasFile:b added:s inCinemas:s digitalRelease:s")
	if m["minimumAvailability"] != "announced" || m["qualityProfileId"].(float64) != float64(hd) || m["isAvailable"] != true {
		t.Fatalf("added %v", m)
	}
	id := int64(m["id"].(float64))
	e.json("POST", radarr+"/movie", add, 200, nil)
	e.json("GET", radarr+"/movie/lookup/tmdb?tmdbId=550", "", 200, &m)
	if int64(m["id"].(float64)) != id {
		t.Fatalf("lookup marks the library movie %v", m)
	}
	if code, _ := e.do("GET", radarr+"/movie/lookup/tmdb?tmdbId=1", ""); code != 404 {
		t.Fatalf("unknown tmdb: %d", code)
	}
	sd := e.profileID(radarr, "Any")
	e.json("PUT", radarr+"/movie", fmt.Sprintf(`{"id":%d,"monitored":false,"qualityProfileId":%d,"minimumAvailability":"inCinemas","rootFolderPath":%q}`,
		id, sd, e.movDir), 202, &m)
	if m["monitored"] != false || m["minimumAvailability"] != "inCinemas" || m["qualityProfileId"].(float64) != float64(sd) {
		t.Fatalf("edited %v", m)
	}
	e.json("PUT", fmt.Sprintf("%s/movie/%d", radarr, id), `{"monitored":true}`, 202, &m)
	var list []map[string]any
	e.json("GET", radarr+"/movie?tmdbId=550", "", 200, &list)
	if len(list) != 1 || list[0]["monitored"] != true {
		t.Fatalf("list %v", list)
	}
	e.json("GET", fmt.Sprintf("%s/movie/%d", radarr, id), "", 200, nil)

	var cmd struct {
		ID int64 `json:"id"`
	}
	e.json("POST", radarr+"/command", fmt.Sprintf(`{"name":"MoviesSearch","movieIds":[%d]}`, id), 201, &cmd)
	e.json("POST", radarr+"/command", `{"name":"MoviesSearch","movieIds":[777]}`, 201, nil)
	e.waitBackground()
	if len(e.movies.ids) != 1 || e.movies.ids[0] != id {
		t.Fatalf("searches %v", e.movies.ids)
	}
	var st struct {
		Status string `json:"status"`
	}
	e.json("GET", fmt.Sprintf("%s/command/%d", radarr, cmd.ID+1), "", 200, &st)
	if st.Status != "failed" {
		t.Fatalf("search of an unknown movie: %+v", st)
	}

	e.grab(domain.KindMovie, "mmm", domain.StateImportBlocked, domain.ItemRef{Kind: domain.KindMovie, ID: id})
	e.grab(domain.KindMovie, "nnn", domain.StateQueued)
	var q map[string]any
	e.json("GET", radarr+"/queue?includeMovie=true", "", 200, &q)
	recs := q["records"].([]any)
	if q["totalRecords"].(float64) != 2 || len(recs) != 2 {
		t.Fatalf("queue %v", q)
	}
	for _, r := range recs {
		rec := r.(map[string]any)
		if rec["downloadId"] == "mmm" && (rec["movieId"].(float64) != float64(id) || rec["movie"] == nil || rec["trackedDownloadState"] != "importBlocked") {
			t.Fatalf("movie record %v", rec)
		}
	}
	var details []any
	e.json("GET", radarr+"/queue/details", "", 200, &details)
	var qs map[string]any
	e.json("GET", radarr+"/queue/status", "", 200, &qs)
	if len(details) != 2 || qs["totalCount"].(float64) != 2 {
		t.Fatalf("details %v status %v", details, qs)
	}

	var cal []map[string]any
	e.json("GET", radarr+"/calendar?start="+aired.Add(-time.Hour).Format(time.RFC3339)+"&end="+aired.Add(time.Hour).Format(time.RFC3339), "", 200, &cal)
	if len(cal) != 1 {
		t.Fatalf("calendar %v", cal)
	}
	e.json("GET", radarr+"/calendar", "", 200, &cal)
	if len(cal) != 0 {
		t.Fatalf("nothing released today %v", cal)
	}
}

func TestRadarrErrors(t *testing.T) {
	e := newEnv(t, "", config.LegacyRadarr)
	for body, want := range map[string]int{
		`{"tmdbId":550,"qualityProfileId":999}`:           400,
		`{"tmdbId":0}`:                                    400,
		`{"tmdbId":550,"minimumAvailability":"whenever"}`: 400,
		`{"tmdbId":1}`:                                    404,
		`{`:                                               400,
	} {
		if code, out := e.do("POST", radarr+"/movie", body); code != want {
			t.Errorf("POST %s: %d %s", body, code, out)
		}
	}
	m, _, err := e.Movies.Add(ctx, movies.AddRequest{TMDBID: 550})
	if err != nil {
		t.Fatal(err)
	}
	for path, c := range map[string][2]any{
		"/movie/42":                        {`{}`, 404},
		fmt.Sprintf("/movie/%d", m.ID):     {`{"qualityProfileId":999}`, 400},
		"/movie/0":                         {`{}`, 404},
		fmt.Sprintf("/movie/%d?x=1", m.ID): {`{"minimumAvailability":"never"}`, 400},
	} {
		if code, _ := e.do("PUT", radarr+path, c[0].(string)); code != c[1].(int) {
			t.Errorf("PUT %s: %d", path, code)
		}
	}
	if code, _ := e.do("GET", radarr+"/movie/42", ""); code != 404 {
		t.Errorf("unknown movie: %d", code)
	}
	if code, _ := e.do("POST", radarr+"/command", `{"name":"MoviesSearch"}`); code != 400 {
		t.Errorf("no ids: %d", code)
	}
	e.mode.Store(domain.ModeDisabled)
	if code, _ := e.do("POST", radarr+"/movie", `{"tmdbId":550}`); code != 409 {
		t.Errorf("disabled movies: %d", code)
	}
}

// ── Lidarr and Prowlarr ──────────────────────────────────────────────────────

func TestLidarrAndProwlarrReads(t *testing.T) {
	e := newEnv(t, "", config.LegacyLidarr, config.LegacyProwlarr)
	_, body := e.do("GET", "/legacy/lidarr/api/v1/artist", "")
	hasTypes(t, fields(t, body), "id:n artistName:s foreignArtistId:s monitored:b qualityProfileId:n tags:a")
	_, body = e.do("GET", "/legacy/lidarr/api/v1/wanted/missing", "")
	page := fields(t, body)
	if page["totalRecords"].(float64) != 1 {
		t.Fatalf("wanted albums %s", body)
	}
	hasTypes(t, page["records"].([]any)[0].(map[string]any), "id:n artistId:n title:s foreignAlbumId:s monitored:b artist:o releaseDate:s")
	e.grab(domain.KindMusic, "mus", domain.StateFailed, domain.ItemRef{Kind: domain.KindMusic, ID: 5})
	var st map[string]any
	e.json("GET", "/legacy/lidarr/api/v1/queue/status", "", 200, &st)
	if st["totalCount"].(float64) != 1 || st["errors"] != true {
		t.Fatalf("music queue %v", st)
	}

	var ixs []map[string]any
	e.json("GET", "/legacy/prowlarr/api/v1/indexer", "", 200, &ixs)
	if len(ixs) != 2 || ixs[0]["name"] != "Fast" || ixs[0]["enable"] != true || ixs[1]["name"] != "slow" || ixs[1]["enable"] != false {
		t.Fatalf("indexers %v", ixs)
	}
	hasTypes(t, ixs[0], "id:n name:s enable:b protocol:s priority:n")
	for range 3 {
		if err := e.st.Searches().RecordQuery(ctx, "fast", day0); err != nil {
			t.Fatal(err)
		}
	}
	e.grab(domain.KindMovie, "g1", domain.StateFailed)
	e.grab(domain.KindMovie, "g2", domain.StateDownloading)
	var stats struct {
		Indexers []map[string]any `json:"indexers"`
	}
	e.json("GET", "/legacy/prowlarr/api/v1/indexerstats", "", 200, &stats)
	fast := stats.Indexers[0]
	hasTypes(t, fast, "indexerId:n indexerName:s numberOfGrabs:n numberOfQueries:n numberOfFailedGrabs:n numberOfFailedQueries:n")
	if fast["indexerId"] != ixs[0]["id"] || fast["numberOfQueries"].(float64) != 3 || fast["numberOfGrabs"].(float64) != 3 ||
		fast["numberOfFailedGrabs"].(float64) != 2 || stats.Indexers[1]["numberOfGrabs"].(float64) != 0 {
		t.Fatalf("stats %v", stats.Indexers)
	}
}
