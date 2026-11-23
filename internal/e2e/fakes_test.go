package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	movieTitle   = "Example Movie"
	releaseTitle = "Example.Movie.2021.1080p.WEB-DL.x264-GRP"
	infoHash     = "0123456789abcdef0123456789abcdef01234567"
	// altTitle is a clean second release served once a search has run, so a
	// re-search after a rejection has something to grab.
	altTitle = "Example.Movie.2021.1080p.WEB-DL.x264-ALT"
	altHash  = "89abcdef0123456789abcdef0123456789abcdef"
	// showPack is the season pack the TV searches find.
	showTitle = "Example Show"
	showPack  = "Example.Show.S01.1080p.WEB-DL.x264-GRP"
	showHash  = "fedcba9876543210fedcba9876543210fedcba98"
)

// releaseOf names each fake torrent by its info hash.
var releaseOf = map[string]string{infoHash: releaseTitle, altHash: altTitle, showHash: showPack}

// categoryOf is the downloader category each fake torrent must arrive with.
var categoryOf = map[string]string{infoHash: "movies", altHash: "movies", showHash: "tv"}

const itemXML = `<item><title>%s</title><guid>guid-%s</guid><link>magnet:?xt=urn:btih:%s&amp;dn=x</link>
<pubDate>Mon, 15 Nov 2021 10:00:00 +0000</pubDate><size>4000000000</size><category>2000</category>
<torznab:attr name="seeders" value="%d"/><torznab:attr name="infohash" value="%s"/></item>`

// fakeTMDB serves one movie, released digitally in 2021.
func fakeTMDB(t *testing.T) *httptest.Server {
	movie := map[string]any{"id": 550, "imdb_id": "tt0000550", "title": movieTitle, "original_title": movieTitle,
		"release_date": "2021-03-01", "runtime": 110, "status": "Released",
		"release_dates": map[string]any{"results": []any{map[string]any{"iso_3166_1": "US", "release_dates": []any{
			map[string]any{"type": 3, "release_date": "2021-03-01T00:00:00.000Z"},
			map[string]any{"type": 4, "release_date": "2021-05-01T00:00:00.000Z"}}}}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/configuration":
			_, _ = w.Write([]byte(`{"images":{"secure_base_url":"https://img.invalid/"}}`))
		case r.URL.Path == "/search/movie":
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{movie}})
		case r.URL.Path == "/movie/550":
			_ = json.NewEncoder(w).Encode(movie)
		case r.URL.Path == "/search/tv":
			_, _ = w.Write([]byte(`{"results":[{"id":77,"name":"` + showTitle + `","first_air_date":"2020-01-05"}]}`))
		case r.URL.Path == "/tv/77":
			_, _ = w.Write([]byte(`{"id":77,"name":"` + showTitle + `","first_air_date":"2020-01-05","status":"Ended",
				"episode_run_time":[30],"number_of_seasons":1,"seasons":[{"season_number":1}],"external_ids":{"tvdb_id":321}}`))
		case r.URL.Path == "/tv/77/season/1":
			_, _ = w.Write([]byte(`{"episodes":[{"season_number":1,"episode_number":1,"name":"Pilot","air_date":"2020-01-05"},
				{"season_number":1,"episode_number":2,"name":"Second","air_date":"2020-01-12"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeTorznab returns the main magnet release for every search and, with alt
// set, the clean alternative from the second search on.
type fakeTorznab struct {
	srv       *httptest.Server
	mu        sync.Mutex
	searches  int
	alt       bool
	tvQueries []string
}

const capsXML = `<caps><searching><search available="yes" supportedParams="q"/>` +
	`<movie-search available="yes" supportedParams="q,imdbid,tmdbid"/>` +
	`<tv-search available="yes" supportedParams="q,tvdbid,season,ep"/></searching>` +
	`<categories><category id="2000" name="Movies"/><category id="5000" name="TV"/></categories></caps>`

func newTorznab(t *testing.T) *fakeTorznab {
	f := &fakeTorznab{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("t") {
		case "caps":
			_, _ = w.Write([]byte(capsXML))
		case "tvsearch":
			f.mu.Lock()
			f.tvQueries = append(f.tvQueries, r.URL.RawQuery)
			f.mu.Unlock()
			items := ""
			if r.URL.Query().Get("ep") == "" && r.URL.Query().Get("tvdbid") == "321" {
				items = fmt.Sprintf(itemXML, showPack, "tv", showHash, 40, showHash)
			}
			_, _ = fmt.Fprintf(w, `<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>%s</channel></rss>`, items)
		case "movie", "search":
			f.mu.Lock()
			f.searches++
			items := fmt.Sprintf(itemXML, releaseTitle, "1", infoHash, 50, infoHash)
			if f.alt && f.searches > 1 {
				items += fmt.Sprintf(itemXML, altTitle, "2", altHash, 10, altHash)
			}
			f.mu.Unlock()
			_, _ = fmt.Fprintf(w, `<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>%s</channel></rss>`, items)
		default:
			_, _ = w.Write([]byte(`<rss><channel></channel></rss>`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTorznab) count() int { f.mu.Lock(); defer f.mu.Unlock(); return f.searches }

// fakeQB is a qBittorrent Web API whose torrents advance one state per
// queue poll: metaDL (no file list yet) → downloading → uploading (done).
type fakeQB struct {
	srv     *httptest.Server
	dir     string   // download dir where payload files appear
	files   []string // payload file names of the main release, relative to its folder
	mu      sync.Mutex
	state   map[string]int // hash → step
	deleted map[string]bool
}

var qbStates = []string{"metaDL", "downloading", "uploading"}

func newQB(t *testing.T, dir string, files []string) *fakeQB {
	f := &fakeQB{dir: dir, files: files, state: map[string]int{}, deleted: map[string]bool{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeQB) serve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/")
	f.mu.Lock()
	defer f.mu.Unlock()
	switch path {
	case "auth/login":
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "s"})
		_, _ = w.Write([]byte("Ok."))
	case "app/version":
		_, _ = w.Write([]byte("v5.0.0"))
	case "torrents/categories":
		_, _ = w.Write([]byte("{}"))
	case "torrents/createCategory":
	case "torrents/add":
		f.add(w, r)
	case "torrents/info":
		f.info(w, r)
	case "torrents/files":
		f.listFiles(w, r)
	case "torrents/delete":
		_ = r.ParseForm()
		f.deleted[r.PostForm.Get("hashes")] = r.PostForm.Get("deleteFiles") == "true"
		delete(f.state, r.PostForm.Get("hashes"))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeQB) add(w http.ResponseWriter, r *http.Request) {
	hash := ""
	for h := range releaseOf {
		if strings.Contains(r.FormValue("urls"), h) {
			hash = h
		}
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil || hash == "" || r.FormValue("category") != categoryOf[hash] {
		_, _ = w.Write([]byte("Fails."))
		return
	}
	f.state[hash] = 0
	for _, name := range f.filesOf(hash) {
		p := filepath.Join(f.dir, releaseOf[hash], name)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(strings.Repeat("v", 4096)), 0o644)
	}
	_, _ = w.Write([]byte("Ok."))
}

// info answers lookups by hash without side effects; a category listing is a
// queue poll and advances every torrent one step.
func (f *fakeQB) info(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	for hash, step := range f.state {
		if r.URL.Query().Get("category") != "" && step < len(qbStates)-1 {
			step++
			f.state[hash] = step
		}
		out = append(out, map[string]any{"hash": hash, "name": releaseOf[hash], "category": categoryOf[hash], "state": qbStates[step],
			"progress": float64(step) / 2, "size": 4096, "save_path": f.dir, "content_path": filepath.Join(f.dir, releaseOf[hash])})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (f *fakeQB) listFiles(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	hash := r.URL.Query().Get("hash")
	if step, ok := f.state[hash]; ok && step > 0 {
		for _, name := range f.filesOf(hash) {
			out = append(out, map[string]any{"name": releaseOf[hash] + "/" + name, "size": 4096})
		}
	}
	_ = json.NewEncoder(w).Encode(out)
}

// filesOf is the payload of a torrent: the alternative is always clean.
func (f *fakeQB) filesOf(hash string) []string {
	switch hash {
	case altHash:
		return []string{"movie.mkv"}
	case showHash:
		return []string{"Example.Show.S01E01.1080p.WEB-DL.x264-GRP.mkv", "Example.Show.S01E02.1080p.WEB-DL.x264-GRP.mkv"}
	}
	return f.files
}

func (f *fakeQB) added(hash string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.state[hash]
	return ok
}

func (f *fakeQB) deletedWithFiles() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deleted[infoHash]
}

// fakePlex records partial refreshes.
type fakePlex struct {
	srv   *httptest.Server
	mu    sync.Mutex
	paths []string
}

func newPlex(t *testing.T, shows string, libraries ...string) *fakePlex {
	f := &fakePlex{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			_, _ = w.Write([]byte(`{"MediaContainer":{"machineIdentifier":"m1","version":"1"}}`))
		case "/library/sections":
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Directory": []any{
				map[string]any{"key": "1", "title": "Movies", "type": "movie", "Location": locations(libraries)},
				map[string]any{"key": "2", "title": "TV", "type": "show", "Location": []any{map[string]any{"path": shows}}}}}})
		case "/library/sections/1/refresh", "/library/sections/2/refresh":
			f.mu.Lock()
			f.paths = append(f.paths, r.URL.Query().Get("path"))
			f.mu.Unlock()
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func locations(paths []string) []any {
	out := make([]any, len(paths))
	for i, p := range paths {
		out[i] = map[string]any{"path": p}
	}
	return out
}

func (f *fakePlex) refreshed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...)
}

// fakeNtfy counts published messages.
type fakeNtfy struct {
	srv    *httptest.Server
	mu     sync.Mutex
	titles map[string][]string // topic path → titles
}

func newNtfy(t *testing.T) *fakeNtfy {
	f := &fakeNtfy{titles: map[string][]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.titles[r.URL.Path] = append(f.titles[r.URL.Path], r.Header.Get("Title"))
		f.mu.Unlock()
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeNtfy) messages(topic string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.titles[topic]...)
}
