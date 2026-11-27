package mcp

import (
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

func TestFindMediaAcrossKinds(t *testing.T) {
	h := newHarness(t)
	out := h.ok(t, "find_media", map[string]any{"query": "alpha"}, "[movie] Alpha (2020) — in library tmdb=10 imdb=tt1", "[tv] Show", "[music] Band")
	cands := out.Data["candidates"].([]any)
	if first := cands[0].(map[string]any); first["libraryId"] != float64(1) || !strings.HasSuffix(first["overview"].(string), "…") {
		t.Fatalf("%v", first)
	}
	h.modes[domain.KindTV] = domain.ModeDisabled
	if out := h.ok(t, "find_media", map[string]any{"query": "x"}); strings.Contains(out.Text, "[tv]") {
		t.Fatal("disabled kinds are skipped")
	}
	h.ok(t, "find_media", map[string]any{"query": "x", "kind": "tv"}, "note: tv: refused")
	h.ok(t, "find_media", map[string]any{"query": "x", "kind": "book"}, "note: book: unsupported")
	h.fails(t, "find_media", map[string]any{"query": " "}, "query: is required")
	h.fails(t, "find_media", map[string]any{"query": 5}, "arguments:")
	h.mv.err = errBoom
	h.ok(t, "find_media", map[string]any{"query": "x", "kind": "movie"}, "note: movie: boom")
}

func TestFindMediaMusicLevels(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "find_media", map[string]any{"query": "band", "kind": "music"}, "[music] Band", "mbid=a0000000")
	out := h.ok(t, "find_media", map[string]any{"query": "rec", "kind": "music", "level": "album", "artist": "Band"}, "Record by Band (2021)")
	if strings.Contains(out.Text, "Other") {
		t.Fatal("albums filter by title")
	}
	h.ok(t, "find_media", map[string]any{"query": "song", "kind": "music", "level": "track", "artist": "a0000000-0000-4000-8000-000000000001"},
		"Song by Band", "request_music with track")
	if !h.c.has("music.lookup mbid:a0000000-0000-4000-8000-000000000001") {
		t.Fatal("an mbid artist is looked up by id")
	}
	h.ok(t, "find_media", map[string]any{"query": "x", "kind": "music", "level": "album"}, "need the artist")
	h.ok(t, "find_media", map[string]any{"query": "x", "kind": "music", "level": "track", "artist": "nobody"}, "no artist matches nobody")
	h.ok(t, "find_media", map[string]any{"query": "Nowhere", "kind": "music", "level": "track", "artist": "Band"}, "holds that track")
	h.ok(t, "find_media", map[string]any{"query": "x", "kind": "music", "level": "song"}, "level: must be")
	h.mu.err = errBoom
	h.ok(t, "find_media", map[string]any{"query": "x", "kind": "music", "level": "album", "artist": "Band"}, "boom")
}

func TestRequestMovie(t *testing.T) {
	h := newHarness(t)
	out := h.ok(t, "request_movie", map[string]any{"tmdbId": 10}, "already in the library: Alpha (movie 1): imported", "/movies/Alpha")
	if out.Data["created"] == true {
		t.Fatal("idempotent")
	}
	h.ok(t, "request_movie", map[string]any{"imdbId": "tt1"}, "already in the library: Alpha")
	if !h.c.has("movies.lookup imdb:tt1") {
		t.Fatal("the IMDb id is resolved through the lookup")
	}
	out = h.ok(t, "request_movie", map[string]any{"tmdbId": 30}, "requested: Added (movie 3): searching")
	if out.Data["created"] != true {
		t.Fatalf("%v", out.Data)
	}
	h.fails(t, "request_movie", map[string]any{}, "give tmdbId or imdbId")
	h.fails(t, "request_movie", map[string]any{"imdbId": "none"}, "no movie with IMDb id none")
	h.modes[domain.KindMovie] = domain.ModeDisabled
	h.fails(t, "request_movie", map[string]any{"tmdbId": 10}, "refused: movie is disabled")
}

func TestRequestShow(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "request_show", map[string]any{"tmdbId": 99, "monitor": "first_season"}, "requested: New Show")
	if !h.c.has("tv.add firstSeason") {
		t.Fatal(h.c.list)
	}
	h.ok(t, "request_show", map[string]any{"tvdbId": 7}, "already in the library: Show (tv 1): downloading", "Show.S01")
	if h.c.has("tv.episodes") {
		t.Fatal("nothing picked, nothing changed")
	}
	h.ok(t, "request_show", map[string]any{"tmdbId": 77, "seasons": []int{2}}, "Show")
	if !h.c.has("tv.episodes") || h.c.has("tv.searchEpisode") == false {
		t.Fatal("an existing series gets the picked season monitored and searched")
	}
	h.c.list = nil
	h.ok(t, "request_show", map[string]any{"tmdbId": 88, "episodes": []map[string]int{{"season": 1, "episode": 2}}, "search": false}, "requested")
	if !h.c.has("tv.add none") || !h.c.has("tv.episodes") || h.c.has("tv.searchEpisode") {
		t.Fatal("picked episodes are monitored after the add, not searched when search is false")
	}
	h.fails(t, "request_show", map[string]any{"tmdbId": 77, "episodes": []map[string]int{{"season": 9, "episode": 9}}}, "no episode")
	h.fails(t, "request_show", map[string]any{"tmdbId": 77, "monitor": "most"}, "monitor: must be one of")
	h.fails(t, "request_show", map[string]any{}, "give tmdbId or tvdbId")
	h.fails(t, "request_show", map[string]any{"tvdbId": 404}, "no series with TVDB id 404")
	h.tvs.err = errBoom
	h.fails(t, "request_show", map[string]any{"tmdbId": 77, "seasons": []int{1}}, "boom")
	h.modes[domain.KindTV] = domain.ModeDisabled
	h.fails(t, "request_show", map[string]any{"tmdbId": 77}, "tv is disabled")
}

func TestRequestMusic(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "request_music", map[string]any{"artistMbid": "new-artist", "monitor": "future"}, "requested: New Artist")
	if !h.c.has("music.add future") {
		t.Fatal(h.c.list)
	}
	out := h.ok(t, "request_music", map[string]any{"artistMbid": "artist-1", "albumMbid": "album-1"}, "requested: Band - Record (music 1): imported")
	if out.Data["albumId"] != float64(100) || !h.c.has("music.searchAlbum") || !h.c.has("music.add none") {
		t.Fatalf("%v %v", out.Data, h.c.list)
	}
	h.ok(t, "request_music", map[string]any{"artistMbid": "artist-1", "albumMbid": "album-1"}, "already in the library")
	h.ok(t, "request_music", map[string]any{"artistMbid": "artist-1", "track": "Song", "search": false}, `"Song" is on Album "Record"`)
	h.fails(t, "request_music", map[string]any{"artistMbid": "artist-1", "track": "Nowhere"}, "single-track downloads are not supported")
	h.fails(t, "request_music", map[string]any{"artistMbid": "artist-1", "albumMbid": "album-9"}, "no release group album-9")
	h.fails(t, "request_music", map[string]any{"artistMbid": "artist-1", "monitor": "some"}, "monitor: must be")
	h.fails(t, "request_music", map[string]any{}, "artistMbid: is required")
	h.mu.err = errBoom
	h.fails(t, "request_music", map[string]any{"artistMbid": "artist-1", "track": "Song"}, "boom")
	h.fails(t, "request_music", map[string]any{"artistMbid": "artist-1", "albumMbid": "album-1"}, "boom")
	h.fails(t, "request_music", map[string]any{"artistMbid": "artist-1"}, "boom")
	h.modes[domain.KindMusic] = domain.ModeDisabled
	h.fails(t, "request_music", map[string]any{"artistMbid": "artist-1"}, "music is disabled")
}

func TestRequestBookIsRefusedWithTheReason(t *testing.T) {
	h := newHarness(t)
	h.fails(t, "request_book", map[string]any{"isbn": "1"}, "book is disabled")
	h.modes[domain.KindBook] = domain.ModeActive
	h.fails(t, "request_book", map[string]any{"isbn": "1"}, "books are not supported")
}

func TestRequestStatusStates(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "request_status", map[string]any{"kind": "movie", "imdbId": "tt1"}, "imported")
	h.ok(t, "request_status", map[string]any{"kind": "movie", "tmdbId": 20}, "all_rejected")
	h.ok(t, "request_status", map[string]any{"kind": "tv", "tvdbId": 7}, `downloading — grabbed "Show.S01" on qbit: downloading, 50%`)
	h.ok(t, "request_status", map[string]any{"kind": "tv", "imdbId": "tt7"}, "downloading")
	h.ok(t, "request_status", map[string]any{"kind": "music", "mbid": "artist-1"}, "Band (music 1): searching")
	h.ok(t, "request_status", map[string]any{"kind": "music", "mbid": "album-1"}, "Band - Record", "imported")
	h.fails(t, "request_status", map[string]any{"kind": "music"}, "give id, albumId or mbid")
	h.fails(t, "request_status", map[string]any{"kind": "movie", "tmdbId": 404}, "not in the library")
	h.fails(t, "request_status", map[string]any{"kind": "tv", "tmdbId": 404}, "not in the library")
	h.fails(t, "request_status", map[string]any{"kind": "music", "mbid": "nope"}, "not in the library")
	h.fails(t, "request_status", map[string]any{"kind": "game", "id": 1}, "kind: must be")
	h.fails(t, "request_status", map[string]any{"kind": "music", "id": 9}, "not found")
	h.fails(t, "request_status", map[string]any{"kind": "music", "albumId": 9}, "not found")
	h.fails(t, "request_status", map[string]any{"kind": "tv", "id": 9}, "not found")
	h.q.err = errBoom
	h.fails(t, "request_status", map[string]any{"kind": "movie", "id": 1}, "boom")
	h.mv.err = errBoom
	h.fails(t, "request_status", map[string]any{"kind": "movie", "tmdbId": 1}, "boom")
}

func TestJudge(t *testing.T) {
	at := day
	for want, st := range map[string]Status{
		StateShadowed:    {Mode: domain.ModeShadow, Monitored: true},
		StateUnmonitored: {},
		StatePartial:     {Monitored: true, Have: 1, Wanted: 3},
		StateRejected:    {Monitored: true, LastSearch: &domain.SearchState{LastSearched: &at, LastResult: &domain.SearchEventData{Releases: 4}}},
		StateWanted:      {Monitored: true, LastSearch: &domain.SearchState{LastSearched: &at, LastResult: &domain.SearchEventData{Releases: 4, Accepted: 2}}},
		StateSearching:   {Monitored: true, LastSearch: &domain.SearchState{LastSearched: &at}},
		StateNotFound:    {Monitored: true, LastSearch: &domain.SearchState{LastSearched: &at, LastResult: &domain.SearchEventData{}}},
	} {
		judge(&st)
		if st.State != want || st.Reason == "" {
			t.Errorf("%s: got %s", want, st.State)
		}
	}
}

func TestDownloadScanIsBounded(t *testing.T) {
	h := newHarness(t)
	page := make([]store.StoredDownload, MaxLimit)
	h.q.pages = nil
	for i := 0; i < maxQueueScan/MaxLimit+2; i++ {
		h.q.pages = append(h.q.pages, page)
	}
	seen := 0
	if err := h.s.eachDownload(t.Context(), func(store.StoredDownload) { seen++ }); err != nil || seen != maxQueueScan {
		t.Fatal(seen, err)
	}
}

func TestCancelRequest(t *testing.T) {
	h := newHarness(t)
	out := h.ok(t, "cancel_request", map[string]any{"kind": "tv", "id": 1, "deleteFiles": true}, "unmonitored, 1 downloads removed")
	if !h.c.has("tv.update") || !h.c.has("queue.remove+files") || out.Data["removedDownloads"] != float64(1) {
		t.Fatal(h.c.list)
	}
	h.ok(t, "cancel_request", map[string]any{"kind": "movie", "id": 2}, "0 downloads removed")
	h.ok(t, "cancel_request", map[string]any{"kind": "music", "albumId": 100}, "cancelled Band - Record")
	h.ok(t, "cancel_request", map[string]any{"kind": "music", "id": 1}, "cancelled Band")
	if !h.c.has("music.update") {
		t.Fatal(h.c.list)
	}
	h.fails(t, "cancel_request", map[string]any{"kind": "movie", "id": 404}, "not found")
	h.fails(t, "cancel_request", map[string]any{"kind": "movie"}, "not in the library")
	h.q.err = errBoom
	h.fails(t, "cancel_request", map[string]any{"kind": "tv", "id": 1}, "boom")
	if n, err := h.s.removeDownloads(t.Context(), []DownloadView{{ID: 1}}, false); n != 0 || err == nil {
		t.Fatal("remove errors stop")
	}
}
