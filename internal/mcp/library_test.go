package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
)

func TestSearchLibraryPagesAcrossKinds(t *testing.T) {
	h := newHarness(t)
	out := h.ok(t, "search_library", map[string]any{"limit": 2}, "2 library items (more: pass cursor \"2\")")
	h.ok(t, "search_library", map[string]any{"cursor": out.Data["nextCursor"]}, "2 library items", "Show", "Band")
	h.ok(t, "search_library", map[string]any{"query": "ALP", "kind": "movie"}, "1 library items")
	h.fails(t, "search_library", map[string]any{"cursor": "x"}, "cursor:")
	h.mv.err = errBoom
	h.fails(t, "search_library", map[string]any{"kind": "movie"}, "boom")
	h.modes[domain.KindTV] = domain.ModeDisabled
	h.fails(t, "search_library", map[string]any{"kind": "tv"}, "disabled")
}

func TestGetItem(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "get_item", map[string]any{"kind": "movie", "id": 1}, "Alpha (2020)", "alpha.mkv")
	out := h.ok(t, "get_item", map[string]any{"kind": "tv", "id": 1, "season": 2}, "Show (2019): 2 seasons")
	if eps := out.Data["episodes"].([]any); len(eps) != 1 {
		t.Fatalf("one season's episodes: %v", eps)
	}
	if out := h.ok(t, "get_item", map[string]any{"kind": "tv", "id": 1}); out.Data["episodes"] != nil {
		t.Fatal("episodes only for a season")
	}
	h.ok(t, "get_item", map[string]any{"kind": "music", "id": 1}, "Band: 1 albums")
	h.ok(t, "get_item", map[string]any{"kind": "music", "albumId": 100}, "Band - Record")
	h.fails(t, "get_item", map[string]any{"kind": "movie", "id": 9}, "not found")
	h.fails(t, "get_item", map[string]any{"kind": "tv", "id": 9}, "not found")
	h.fails(t, "get_item", map[string]any{"kind": "x"}, "kind")
}

func TestListWanted(t *testing.T) {
	h := newHarness(t)
	idx := h.s.d.MovieSearch.Index()
	idx.Put(automation.Entry{Movie: domain.Movie{ID: 2, Title: "Beta"}, Reason: automation.ReasonMissing})
	h.ok(t, "list_wanted", map[string]any{"kind": "movie"}, "1 wanted movie items (missing)", "Beta")
	h.ok(t, "list_wanted", map[string]any{"kind": "movie", "reason": "cutoff_unmet"}, "0 wanted movie items (cutoff_unmet)")
	h.ok(t, "list_wanted", map[string]any{"kind": "tv"}, "0 wanted tv items")
	h.ok(t, "list_wanted", map[string]any{"kind": "music"}, "0 wanted music items")
	h.fails(t, "list_wanted", map[string]any{"kind": "movie", "cursor": "-1"}, "cursor")
	h.fails(t, "list_wanted", map[string]any{}, "kind")
}

func TestCalendar(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "get_calendar", nil, "1 episodes and 1 movie releases between 2026-03-10 and 2026-03-17")
	h.ok(t, "get_calendar", map[string]any{"from": "2026-04-01", "to": "2026-05-01"}, "1 movie releases")
	h.fails(t, "get_calendar", map[string]any{"from": "soon"}, "from: must be a date")
	h.modes[domain.KindTV], h.modes[domain.KindMovie] = domain.ModeDisabled, domain.ModeDisabled
	h.ok(t, "get_calendar", nil, "0 episodes and 0 movie releases")
	h.modes[domain.KindTV] = domain.ModeActive
	h.tvs.err = errBoom
	h.fails(t, "get_calendar", nil, "boom")
	h.modes[domain.KindTV], h.modes[domain.KindMovie] = domain.ModeDisabled, domain.ModeActive
	h.mv.err = errBoom
	h.fails(t, "get_calendar", nil, "boom")
}

func TestHistoryHealthProfilesRoots(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "get_history", map[string]any{"type": "grab"}, "1 history events (more: pass cursor \"1\")")
	h.fails(t, "get_history", map[string]any{"cursor": "z"}, "cursor")
	out := h.ok(t, "get_health", nil, "2 integrations, 1 not ok; 1 root folders")
	if modes := out.Data["modes"].(map[string]any); modes["book"] != "disabled" || modes["movie"] != "active" {
		t.Fatal(modes)
	}
	h.ok(t, "list_profiles", map[string]any{"kind": "movie"}, "quality profiles")
	if out := h.ok(t, "list_profiles", nil); out.Data["metadataProfiles"] == nil {
		t.Fatal("music metadata profiles")
	}
	h.ok(t, "list_root_folders", nil, "1 root folders", "/movies")
	h.s.d.RootFolders = fakeRoots{err: errBoom}
	h.fails(t, "get_health", nil, "boom")
}

func TestSearchReleasesPreviewsWithoutGrabbing(t *testing.T) {
	h := newHarness(t)
	out := h.ok(t, "search_releases", map[string]any{"kind": "movie", "id": 2, "limit": 1}, "2 releases, 1 accepted", "nothing was grabbed")
	if out.Data["omitted"] != float64(1) || len(out.Data["errors"].([]any)) != 1 {
		t.Fatalf("bounded: %v", out.Data)
	}
	h.ok(t, "search_releases", map[string]any{"kind": "tv", "id": 11})
	h.ok(t, "search_releases", map[string]any{"kind": "tv", "seriesId": 1, "season": 1})
	h.ok(t, "search_releases", map[string]any{"kind": "music", "id": 100})
	for _, c := range []string{"movies.interactive", "tv.interactiveEpisode", "tv.interactiveSeason", "music.interactive"} {
		if !h.c.has(c) {
			t.Errorf("%s not called", c)
		}
	}
	if h.c.has("movies.search") {
		t.Fatal("previews never search to grab")
	}
	h.modes[domain.KindMovie] = domain.ModeDisabled
	h.fails(t, "search_releases", map[string]any{"kind": "movie", "id": 2}, "disabled")
}

func TestSetMonitored(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "set_monitored", map[string]any{"kind": "movie", "id": 1, "monitored": false}, "unmonitored Alpha")
	h.ok(t, "set_monitored", map[string]any{"kind": "tv", "id": 1, "monitored": true}, "monitored Show")
	h.ok(t, "set_monitored", map[string]any{"kind": "tv", "id": 1, "season": 2, "monitored": true}, "season 2 of series 1")
	h.ok(t, "set_monitored", map[string]any{"kind": "tv", "episodeIds": []int{11, 12}, "monitored": true}, "2 episodes")
	h.ok(t, "set_monitored", map[string]any{"kind": "music", "albumIds": []int{100}, "monitored": true}, "1 albums")
	h.ok(t, "set_monitored", map[string]any{"kind": "music", "id": 1, "monitored": false}, "artist 1")
	h.fails(t, "set_monitored", map[string]any{"kind": "movie", "id": 9, "monitored": true}, "not found")
	h.fails(t, "set_monitored", map[string]any{"kind": "x"}, "kind")
}

func TestStartSearchAndRescan(t *testing.T) {
	h := newHarness(t)
	for _, args := range []map[string]any{
		{"kind": "movie", "id": 1}, {"kind": "tv", "episodeId": 11}, {"kind": "tv", "id": 1, "season": 1}, {"kind": "tv", "id": 1},
		{"kind": "music", "albumId": 100}, {"kind": "music", "id": 1},
	} {
		h.ok(t, "start_search", args, "search started")
	}
	for _, c := range []string{"movies.search", "tv.searchEpisode", "tv.searchSeason", "tv.searchSeries", "music.searchAlbum", "music.searchArtist"} {
		if !h.c.has(c) {
			t.Errorf("%s not run", c)
		}
	}
	h.fails(t, "start_search", map[string]any{"kind": "movie"}, "give id")
	h.modes[domain.KindMovie] = domain.ModeShadow
	h.ok(t, "start_search", map[string]any{"kind": "movie", "id": 1}, "shadow mode")
	h.fails(t, "start_search", map[string]any{"kind": "x"}, "kind")

	h.ok(t, "rescan_item", map[string]any{"kind": "movie"}, "refresh started")
	h.ok(t, "rescan_item", map[string]any{"kind": "tv", "id": 1}, "refresh started")
	h.ok(t, "rescan_item", map[string]any{"kind": "music", "id": 1}, "refresh started")
	if !h.c.has("movies.refresh") || !h.c.has("tv.refresh") || !h.c.has("music.refresh") {
		t.Fatal(h.c.list)
	}
	h.fails(t, "rescan_item", map[string]any{"kind": "tv"}, "give the series")
	h.fails(t, "rescan_item", map[string]any{"kind": "x"}, "kind")
}

func TestToolHelpers(t *testing.T) {
	if p := (paging{Limit: 1000}); p.size() != MaxLimit {
		t.Fatal(p.size())
	}
	if pg, _ := slicePage([]int{1, 2, 3}, paging{Cursor: "9"}); len(pg.Items) != 0 || pg.NextCursor != "" {
		t.Fatal(pg)
	}
	if res, err := bind(func(_ context.Context, in struct{ A int }) (Result, error) { return Result{Text: "x"}, nil })(t.Context(), json.RawMessage("null")); err != nil || res.Text != "x" {
		t.Fatal(err)
	}
	if !strings.Contains(errText(unsupported("nope")), "unsupported: nope") {
		t.Fatal(errText(unsupported("nope")))
	}
	if short("abc") != "abc" || buildVersion() == "" {
		t.Fatal("helpers")
	}
	if k, err := kindOf("tv"); err != nil || k != domain.KindTV {
		t.Fatal(k, err)
	}
}
