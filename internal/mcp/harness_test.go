package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/events"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/musicauto"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/tv"
	"github.com/jrand0m/corsarr/internal/tvauto"
)

type harness struct {
	s      *Service
	c      *calls
	cfg    atomic.Pointer[config.Config]
	modes  map[domain.MediaKind]domain.KindMode
	mv     *fakeMovies
	tvs    *fakeTV
	mu     *fakeMusic
	q      *fakeQueue
	dl     *fakeDownloaders
	bus    *events.Bus
	sess   *Session
	now    time.Time
	nextID int
}

var (
	day       = time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	sixDays   = day.Add(6 * 24 * time.Hour)
	monthAway = day.Add(40 * 24 * time.Hour)
)

func newHarness(t *testing.T) *harness {
	t.Helper()
	c := &calls{}
	h := &harness{c: c, now: day, modes: map[domain.MediaKind]domain.KindMode{domain.KindBook: domain.ModeDisabled}}
	h.setMCP(config.MCP{Enabled: true, Scope: config.MCPScopeWrite})
	h.mv = &fakeMovies{c: c, movies: []domain.Movie{
		{ID: 1, TMDBID: 10, IMDBID: "tt1", Title: "Alpha", Year: 2020, Monitored: true, FileID: 5, Digital: &sixDays},
		{ID: 2, TMDBID: 20, Title: "Beta", Year: 2021, Monitored: true, InCinemas: &monthAway},
	}, recs: []plugin.MovieRecord{{IDs: map[string]string{"tmdb": "10", "imdb": "tt1"}, Title: "Alpha", Year: 2020, Overview: strings.Repeat("x", 400)},
		{IDs: map[string]string{"tmdb": "30"}, Title: "Gamma"}}}
	h.tvs = &fakeTV{c: c, series: []domain.Series{{ID: 1, TMDBID: 77, TVDBID: 7, IMDBID: "tt7", Title: "Show", Year: 2019, Monitored: true}},
		episodes: []domain.Episode{{ID: 11, SeriesID: 1, Season: 1, Number: 1}, {ID: 12, SeriesID: 1, Season: 1, Number: 2}, {ID: 21, SeriesID: 1, Season: 2, Number: 1}},
		lookup:   []tv.LookupItem{{SeriesRecord: plugin.SeriesRecord{IDs: map[string]string{"tmdb": "77", "tvdb": "7"}, Title: "Show", Year: 2019}, LibraryID: 1}}}
	first := day.AddDate(-5, 0, 0)
	h.mu = &fakeMusic{c: c, artists: []domain.Artist{{ID: 1, MBID: "artist-1", Name: "Band", Monitored: true}},
		albums: []domain.Album{{ID: 100, ArtistID: 1, MBID: "album-1", Title: "Record", TrackCount: 2}},
		lookup: []music.LookupItem{{ArtistRecord: plugin.ArtistRecord{MBID: "a0000000-0000-4000-8000-000000000001", Name: "Band"}, LibraryID: 1}},
		disco: plugin.ArtistRecord{MBID: "artist-1", Name: "Band", Albums: []plugin.AlbumRecord{
			{MBID: "album-1", Title: "Record", PrimaryType: "Album", FirstRelease: &first}, {MBID: "album-2", Title: "Other", PrimaryType: "EP"}}},
		match: music.RecordingMatch{Album: plugin.AlbumRecord{MBID: "album-1", Title: "Record", PrimaryType: "Album"},
			Track: plugin.TrackRecord{RecordingMBID: "rec-1", Title: "Song", Position: 2}}}
	h.q = &fakeQueue{c: c, pages: [][]store.StoredDownload{
		{{Download: domain.Download{ID: 1, Title: "Show.S01", Downloader: "qbit", State: domain.StateDownloading, Progress: 0.5,
			Links: []domain.DownloadLink{{Item: domain.ItemRef{Kind: domain.KindTV, ID: 12}}}}}},
		{{Download: domain.Download{ID: 2, Title: "Other", Links: []domain.DownloadLink{{Item: domain.ItemRef{Kind: domain.KindMovie, ID: 9}}}}}},
	}}
	h.dl = &fakeDownloaders{c: c}
	h.bus = events.New()
	t.Cleanup(h.bus.Close)
	searched := day
	h.s = New(Deps{Config: h.cfg.Load, Modes: func(k domain.MediaKind) domain.KindMode { return h.modes[k] },
		Movies: h.mv, MovieSearch: &fakeMovieSearch{c: c, idx: automation.NewIndex()},
		TV: h.tvs, TVSearch: &fakeTVSearch{c: c, idx: tvauto.NewIndex()},
		Music: h.mu, MusicSearch: &fakeMusicSearch{c: c, idx: musicauto.NewIndex()},
		Queue: h.q, Downloaders: h.dl, Library: fakeLibrary{}, Searches: fakeSearches{states: map[domain.ItemRef]domain.SearchState{
			{Kind: domain.KindMovie, ID: 2}: {LastSearched: &searched, LastResult: &domain.SearchEventData{Releases: 3,
				Rejections: []domain.ReasonCount{{RuleID: "quality.notWanted", Message: "quality not wanted", Count: 3}}}},
		}}, History: fakeHistory{}, Health: fakeHealth{}, RootFolders: fakeRoots{}, Bus: h.bus, Plugins: fakePlugins{},
		Go: func(fn func(context.Context)) { fn(context.Background()) }, Now: func() time.Time { return h.now }})
	h.sess = h.s.NewSession(KeyFull)
	return h
}

func (h *harness) setMCP(m config.MCP) {
	cfg := &config.Config{MCP: m}
	h.cfg.Store(cfg)
}

type rpcOut struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

func (h *harness) rpcOn(t *testing.T, sess *Session, method string, params any) rpcOut {
	t.Helper()
	h.nextID++
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": h.nextID, "method": method, "params": params})
	var out rpcOut
	if err := json.Unmarshal(h.s.Handle(context.Background(), sess, b), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (h *harness) rpc(t *testing.T, method string, params any) rpcOut {
	t.Helper()
	return h.rpcOn(t, h.sess, method, params)
}

type callOut struct {
	Text    string
	Data    map[string]any
	IsError bool
}

func (h *harness) callOn(t *testing.T, sess *Session, name string, args any) callOut {
	t.Helper()
	out := h.rpcOn(t, sess, "tools/call", map[string]any{"name": name, "arguments": args})
	if out.Error != nil {
		t.Fatalf("%s: rpc error %v", name, out.Error)
	}
	var r struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Structured map[string]any `json:"structuredContent"`
		IsError    bool           `json:"isError"`
	}
	_ = json.Unmarshal(out.Result, &r)
	if !r.IsError {
		checkOutput(t, h.s.byName[name], r.Structured)
	}
	return callOut{Text: r.Content[0].Text, Data: r.Structured, IsError: r.IsError}
}

func (h *harness) call(t *testing.T, name string, args any) callOut {
	t.Helper()
	return h.callOn(t, h.sess, name, args)
}

// ok calls a tool that must succeed and whose text must hold every want.
func (h *harness) ok(t *testing.T, name string, args any, want ...string) callOut {
	t.Helper()
	out := h.call(t, name, args)
	if out.IsError {
		t.Fatalf("%s %v: %s", name, args, out.Text)
	}
	for _, w := range want {
		if !strings.Contains(out.Text+fmt.Sprint(out.Data), w) {
			t.Errorf("%s %v: missing %q in %s %v", name, args, w, out.Text, out.Data)
		}
	}
	return out
}

// fails calls a tool that must fail with a text holding want.
func (h *harness) fails(t *testing.T, name string, args any, want string) {
	t.Helper()
	out := h.call(t, name, args)
	if !out.IsError || !strings.Contains(out.Text, want) {
		t.Errorf("%s %v: want error %q, got %v %s", name, args, want, out.IsError, out.Text)
	}
}
