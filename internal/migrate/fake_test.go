package migrate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

const testKey = "k-123-secret"

// fakeArr serves fixture JSON for one source app and records every request.
type fakeArr struct {
	srv     *httptest.Server
	mu      sync.Mutex
	methods []string
	paths   []string
	routes  map[string]any // path (+ "?N" for a seriesId or artistId) → body
	version string
}

func newFake(t *testing.T, app App, routes map[string]any) *fakeArr {
	f := &fakeArr{routes: routes, version: map[App]string{Sonarr: "4.0.20", Radarr: "6.4.4", Lidarr: "3.1.0.4875", Prowlarr: "2.6.5"}[app]}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.methods = append(f.methods, r.Method)
		f.paths = append(f.paths, r.URL.Path)
		f.mu.Unlock()
		if r.Header.Get("X-Api-Key") != testKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/v3"), "/api/v1")
		if path == "/system/status" {
			_ = json.NewEncoder(w).Encode(wStatus{AppName: string(app), InstanceName: "home", Version: f.version})
			return
		}
		for _, parent := range []string{"seriesId", "artistId"} {
			if id := r.URL.Query().Get(parent); id != "" {
				path += "?" + id
			}
		}
		body, ok := f.routes[path]
		if !ok {
			body = []any{}
			if r.URL.Query().Get("page") != "" {
				body = map[string]any{"page": 1, "totalRecords": 0, "records": []any{}}
			}
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeArr) endpoint(t *testing.T, app App) Endpoint {
	ep, err := ParseEndpoint(string(app), f.srv.URL, NewSecret(testKey))
	if err != nil {
		t.Fatal(err)
	}
	return ep
}

func (f *fakeArr) onlyGets(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.methods {
		if m != http.MethodGet {
			t.Fatalf("source received %s", m)
		}
	}
}

func page(records ...any) map[string]any {
	return map[string]any{"page": 1, "pageSize": pageSize, "totalRecords": len(records), "records": records}
}

func field(name string, v any) map[string]any { return map[string]any{"name": name, "value": v} }

// library is a media tree on disk the fixtures point at.
type library struct{ root string }

func newLibrary(t *testing.T) library {
	l := library{root: t.TempDir()}
	l.write(t, "tv/Show/Season 1/Show - S01E01.mkv", 11)
	l.write(t, "movies/Film (2020)/Film.mkv", 22)
	return l
}

func (l library) write(t *testing.T, rel string, size int) {
	p := filepath.Join(l.root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// path is a path as the source reports it (under /data/media).
func src(rel string) string { return "/data/media/" + rel }

func sonarrRoutes(day time.Time) map[string]any {
	return map[string]any{
		"/tag": []any{map[string]any{"id": 1, "label": "kids"}},
		"/qualityprofile": []any{
			map[string]any{"id": 1, "name": "HD-1080p", "cutoff": 7, "items": []any{map[string]any{"quality": map[string]any{"id": 7, "name": "Bluray-1080p"}, "allowed": true}}},
			map[string]any{"id": 2, "name": "Top HDR", "cutoff": 1001, "upgradeAllowed": true,
				"items":       []any{map[string]any{"id": 1001, "name": "WEB 2160p", "items": []any{map[string]any{"quality": map[string]any{"id": 18, "name": "WEBDL-2160p"}}}}},
				"formatItems": []any{map[string]any{"name": "HDR", "score": 50}, map[string]any{"name": "x", "score": 0}}},
		},
		"/customformat":  []any{map[string]any{"id": 1, "name": "HDR"}},
		"/rootfolder":    []any{map[string]any{"id": 1, "path": "/data/media/tv/"}},
		"/config/naming": map[string]any{"renameEpisodes": false},
		"/downloadclient": []any{
			map[string]any{"id": 1, "name": "qBittorrent", "implementation": "QBittorrent", "protocol": "torrent", "enable": true,
				"fields": []any{field("host", "qbit"), field("port", 8080), field("useSsl", false), field("username", "admin"),
					field("password", "********"), field("tvCategory", "tv")}},
			map[string]any{"id": 2, "name": "Bridge", "implementation": "Sabnzbd", "protocol": "usenet", "enable": true,
				"fields": []any{field("host", "bridge"), field("port", 8080)}},
		},
		"/indexer": []any{map[string]any{"id": 3, "name": "Tracker (copy)", "implementation": "Torznab", "protocol": "torrent", "enable": true,
			"fields": []any{field("baseUrl", "http://manager:9696/4/"), field("apiKey", "********")}}},
		"/series": []any{
			map[string]any{"id": 1, "title": "The Show", "year": 2020, "tvdbId": 10, "tmdbId": 100, "path": src("tv/Show"),
				"rootFolderPath": src("tv/"), "qualityProfileId": 2, "monitored": true, "seasonFolder": true, "seriesType": "standard",
				"monitorNewItems": "all", "tags": []int{1}, "status": "continuing", "runtime": 30,
				"seasons": []any{map[string]any{"seasonNumber": 1, "monitored": true}, map[string]any{"seasonNumber": 2, "monitored": false}}},
			map[string]any{"id": 2, "title": "No Ids", "year": 1999, "path": src("tv/No Ids"), "rootFolderPath": src("tv/"), "qualityProfileId": 1},
		},
		"/episode?1": []any{
			map[string]any{"id": 11, "seasonNumber": 1, "episodeNumber": 1, "title": "One", "airDateUtc": day, "monitored": true, "episodeFileId": 501},
			map[string]any{"id": 12, "seasonNumber": 1, "episodeNumber": 2, "title": "Two", "monitored": true, "episodeFileId": 502},
			map[string]any{"id": 13, "seasonNumber": 2, "episodeNumber": 1, "title": "Three", "monitored": false},
		},
		"/episodefile?1": []any{
			map[string]any{"id": 501, "path": src("tv/Show/Season 1/Show - S01E01.mkv"), "size": 11, "releaseGroup": "GRP",
				"quality": map[string]any{"quality": map[string]any{"name": "WEBDL-1080p"}}, "dateAdded": day},
			map[string]any{"id": 502, "path": src("tv/Show/Season 1/Show - S01E02.mkv"), "size": 12,
				"quality": map[string]any{"quality": map[string]any{"name": "Bluray-1080p Remux"}}},
		},
		"/queue": page(
			map[string]any{"seriesId": 1, "episodeId": 11, "title": "Show.S01.1080p", "downloadId": "AAAA", "protocol": "torrent", "size": 100},
			map[string]any{"seriesId": 1, "episodeId": 12, "title": "Show.S01.1080p", "downloadId": "AAAA", "protocol": "torrent", "size": 100},
			map[string]any{"seriesId": 1, "episodeId": 13, "title": "Show.S02E01", "downloadId": "BBBB", "protocol": "torrent", "size": 5},
		),
		"/blocklist": page(map[string]any{"id": 9, "seriesId": 1, "episodeIds": []int{12}, "sourceTitle": "Show.S01E02.BAD",
			"date": day, "indexer": "Tracker", "message": "manually marked as failed"}),
		"/history": page(
			map[string]any{"id": 70, "eventType": "grabbed", "date": day, "sourceTitle": "Show.S01.1080p", "seriesId": 1, "episodeId": 11, "downloadId": "AAAA"},
			map[string]any{"id": 71, "eventType": "seriesFolderImported", "date": day, "sourceTitle": "x", "seriesId": 1},
		),
	}
}

func radarrRoutes(day time.Time) map[string]any {
	return map[string]any{
		"/qualityprofile": []any{map[string]any{"id": 1, "name": "HD-1080p", "cutoff": 7}},
		"/rootfolder":     []any{map[string]any{"id": 1, "path": src("movies")}},
		"/config/naming":  map[string]any{"renameMovies": false},
		"/downloadclient": []any{map[string]any{"id": 1, "name": "qbit", "implementation": "QBittorrent", "protocol": "torrent",
			"enable": true, "fields": []any{field("host", "qbit"), field("port", 8080), field("movieCategory", "movies")}}},
		"/queue":     page(map[string]any{"movieId": 1, "title": "Film.2020.2160p", "downloadId": "CCCC", "protocol": "torrent", "size": 9}),
		"/blocklist": page(map[string]any{"id": 4, "movieId": 2, "sourceTitle": "Later.CAM", "date": day}),
		"/movie": []any{
			map[string]any{"id": 1, "title": "Film", "year": 2020, "tmdbId": 200, "path": src("movies/Film (2020)"), "rootFolderPath": src("movies"),
				"qualityProfileId": 1, "monitored": true, "minimumAvailability": "released",
				"movieFile": map[string]any{"id": 31, "path": src("movies/Film (2020)/Film.mkv"), "size": 22,
					"quality": map[string]any{"quality": map[string]any{"name": "Bluray-1080p"}}, "dateAdded": day}},
			map[string]any{"id": 2, "title": "Later", "year": 2027, "tmdbId": 201, "path": src("movies/Later (2027)"),
				"rootFolderPath": src("movies"), "qualityProfileId": 1, "monitored": true, "minimumAvailability": "tba"},
		},
	}
}

func prowlarrRoutes() map[string]any {
	return map[string]any{
		"/indexer": []any{
			map[string]any{"id": 4, "name": "Public Tracker", "implementation": "Cardigann", "protocol": "torrent", "enable": true, "priority": 25},
			map[string]any{"id": 5, "name": "Own Torznab", "implementation": "Torznab", "protocol": "torrent", "enable": true,
				"fields": []any{field("baseUrl", "http://jackett:9117/torznab"), field("apiPath", "/api"), field("apiKey", "********")}},
			map[string]any{"id": 6, "name": "Bridge Feed", "implementation": "Newznab", "protocol": "usenet", "enable": true,
				"fields": []any{field("baseUrl", "http://bridge:8080")}},
		},
	}
}

// clientFake is a download client listing fixed downloads.
type clientFake struct{ hashes []string }

func (c clientFake) List(context.Context, []string) ([]plugin.Snapshot, error) {
	var out []plugin.Snapshot
	for _, h := range c.hashes {
		out = append(out, plugin.Snapshot{DownloadID: h, Category: "tv", SavePath: "/data/torrents"})
	}
	return out, nil
}

func openStore(t *testing.T) *store.Store {
	st, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
