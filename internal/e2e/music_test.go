package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/testkit"
)

const (
	artistName   = "Example Artist"
	artistMBID   = "a0000000-0000-4000-8000-000000000001"
	albumMBID    = "b0000000-0000-4000-8000-000000000002"
	albumRelease = "Example Artist - First Album (2020) [FLAC]"
	albumHash    = "aaaabbbbccccddddeeeeffff0000111122223333"
)

func init() {
	releaseOf[albumHash] = albumRelease
	categoryOf[albumHash] = "music"
}

// fakeMusicBrainz serves one artist with one album of two 20-minute tracks.
// The search on add races the background edition fetch, so the album may
// be judged with its real length (40 min) or the default one (45 min); the
// indexer's 300 MB FLAC release fits the size limits of both, which keeps
// the decision the same whichever finishes first.
func fakeMusicBrainz(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artist":
			_, _ = fmt.Fprintf(w, `{"artists":[{"id":"%s","name":"%s","sort-name":"Artist, Example","type":"Group"}]}`, artistMBID, artistName)
		case "/artist/" + artistMBID:
			_, _ = fmt.Fprintf(w, `{"id":"%s","name":"%s"}`, artistMBID, artistName)
		case "/release-group":
			_, _ = fmt.Fprintf(w, `{"release-group-count":1,"release-groups":[{"id":"%s","title":"First Album","primary-type":"Album",
				"first-release-date":"2020-03-01"}]}`, albumMBID)
		case "/release":
			_, _ = w.Write([]byte(`{"releases":[{"id":"r1","title":"First Album","status":"Official","date":"2020-03-01",
				"media":[{"position":1,"format":"Digital Media","tracks":[
				{"position":1,"title":"Opening","length":1200000,"recording":{"id":"rec1"}},
				{"position":2,"title":"Closing","length":1200000,"recording":{"id":"rec2"}}]}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeMusicIndexer answers music searches by artist with one FLAC album.
type fakeMusicIndexer struct {
	srv     *httptest.Server
	mu      sync.Mutex
	queries []url.Values
}

func newMusicIndexer(t *testing.T) *fakeMusicIndexer {
	f := &fakeMusicIndexer{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("t") == "caps" {
			_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/>` +
				`<audio-search available="yes" supportedParams="q,artist,album"/></searching>` +
				`<categories><category id="3000" name="Audio"/></categories></caps>`))
			return
		}
		f.mu.Lock()
		f.queries = append(f.queries, q)
		f.mu.Unlock()
		items := ""
		if q.Get("t") == "music" && q.Get("artist") == artistName && q.Get("album") == "First Album" {
			items = fmt.Sprintf(`<item><title>%s</title><guid>guid-music</guid><link>magnet:?xt=urn:btih:%s&amp;dn=x</link>
<pubDate>Mon, 15 Nov 2021 10:00:00 +0000</pubDate><size>300000000</size><category>3000</category>
<torznab:attr name="seeders" value="25"/><torznab:attr name="infohash" value="%s"/></item>`, albumRelease, albumHash, albumHash)
		}
		_, _ = fmt.Fprintf(w, `<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>%s</channel></rss>`, items)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

const musicYAML = `version: 1
mediaKinds:
  movie: { mode: disabled }
  tv: { mode: disabled }
  music: { mode: {{mode}} }
data:
  dir: {{data}}
rootFolders:
  - path: {{music}}
    kind: music
metadata:
  musicbrainz:
    type: musicbrainz
    baseUrl: {{mb}}
    requestsPerSecond: 50
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
`

type musicStack struct {
	*stack
	music string
	ix    *fakeMusicIndexer
}

// startMusic wires the pipeline with only music active: MusicBrainz,
// a music Torznab indexer, qBittorrent, Plex and ntfy are httptest fakes.
func startMusic(t *testing.T, mode domain.KindMode) *musicStack {
	t.Helper()
	base := t.TempDir()
	m := &musicStack{stack: &stack{dl: filepath.Join(base, "downloads")}, music: filepath.Join(base, "media", "music")}
	for _, d := range []string{m.dl, m.music} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{"01 - Opening.flac", "02 - Closing.flac", "cover.jpg"}
	m.ix, m.qb, m.plex, m.ntfy = newMusicIndexer(t), newQB(t, m.dl, files), newPlex(t, filepath.Join(base, "tv"), m.music), newNtfy(t)
	ntfyURL, _ := url.Parse(m.ntfy.srv.URL)
	m.yaml = strings.NewReplacer("{{data}}", filepath.Join(base, "data"), "{{music}}", m.music, "{{mb}}", fakeMusicBrainz(t).URL,
		"{{torznab}}", m.ix.srv.URL, "{{qbit}}", m.qb.srv.URL, "{{plex}}", m.plex.srv.URL, "{{ntfy}}", ntfyURL.Host,
		"{{mode}}", string(mode)).Replace(musicYAML)
	m.path = filepath.Join(base, "corsarr.yml")
	m.cfg.Store(m.load(t, m.yaml))
	m.wire(t)
	return m
}

// The music pipeline: look up an artist → add it with search → album search
// by artist and album → one grab with category music → completion →
// hardlink import of each track into Artist/Album (Year) → Plex refresh of
// the album folder → one ntfy message naming the album → History.
func TestMusicPipelineEndToEnd(t *testing.T) {
	s := startMusic(t, domain.ModeActive)
	if code, body := s.call(t, "GET", "/api/v1/artists/lookup?term=example", ""); code != 200 || !strings.Contains(body, `"libraryId":0`) {
		t.Fatalf("lookup: %d %s", code, body)
	}
	body := fmt.Sprintf(`{"mbid":"%s","monitor":"all","profileKey":"lossless","searchNow":true}`, artistMBID)
	if code, resp := s.call(t, "POST", "/api/v1/artists", body); code != 201 {
		t.Fatalf("add artist: %d %s", code, resp)
	}
	testkit.Eventually(t, 20*time.Second, "import.completed in history", func() bool { return has(s.history(t), "import.completed") })

	album := filepath.Join(s.music, artistName, "First Album (2020)")
	for src, name := range map[string]string{
		"01 - Opening.flac": "Example Artist - First Album - 01 - Opening.flac",
		"02 - Closing.flac": "Example Artist - First Album - 02 - Closing.flac",
	} {
		a, err1 := os.Stat(filepath.Join(s.dl, albumRelease, src))
		b, err2 := os.Stat(filepath.Join(album, name))
		if err1 != nil || err2 != nil || !os.SameFile(a, b) {
			t.Fatalf("hardlink %s: %v %v", name, err1, err2)
		}
	}
	if !s.qb.added(albumHash) {
		t.Fatal("the album was not sent to the downloader")
	}
	s.ix.mu.Lock()
	first := s.ix.queries[0]
	s.ix.mu.Unlock()
	if first.Get("t") != "music" || first.Get("cat") != "3000" {
		t.Fatalf("album search by artist and album: %v", first)
	}
	testkit.Eventually(t, 5*time.Second, "Plex refresh of the album folder", func() bool { return has(s.plex.refreshed(), album) })
	testkit.Eventually(t, 5*time.Second, "one ntfy message", func() bool { return len(s.ntfy.messages("/corsarr")) >= 1 })
	time.Sleep(300 * time.Millisecond)
	if msgs := s.ntfy.messages("/corsarr"); len(msgs) != 1 || msgs[0] != "Imported: Example Artist - First Album" {
		t.Fatalf("ntfy got %v", msgs)
	}
	hist := s.history(t)
	for _, want := range []string{"item.added", "grab.sent", "import.completed"} {
		if !has(hist, want) {
			t.Errorf("history lacks %s: %v", want, hist)
		}
	}
	if n := count(hist, "grab.sent"); n != 1 {
		t.Fatalf("one grab for the album, got %d", n)
	}
	if _, body := s.call(t, "GET", "/api/v1/albums/1", ""); !strings.Contains(body, `"status":"downloaded"`) || strings.Count(body, `"quality":"FLAC"`) < 2 {
		t.Fatalf("album page: %s", body)
	}
	if _, body := s.call(t, "GET", "/api/v1/wanted/missing/albums", ""); strings.Contains(body, "First Album") {
		t.Fatalf("the imported album is no longer missing: %s", body)
	}
	if code, body := s.call(t, "GET", "/api/v1/queue", ""); code != 200 || strings.Contains(body, albumRelease) {
		t.Fatalf("imported download leaves the queue: %s", body)
	}
}

func TestMusicShadowModeDecidesButGrabsNothing(t *testing.T) {
	s := startMusic(t, domain.ModeShadow)
	body := fmt.Sprintf(`{"mbid":"%s","profileKey":"lossless","searchNow":true}`, artistMBID)
	if code, resp := s.call(t, "POST", "/api/v1/artists", body); code != 201 {
		t.Fatalf("add artist: %d %s", code, resp)
	}
	testkit.Eventually(t, 10*time.Second, "decision.shadowed in history", func() bool { return has(s.history(t), "decision.shadowed") })
	// decision.shadowed is recorded by the same call that would have sent
	// the grab, so once it is in History nothing can still be on its way.
	if s.qb.added(albumHash) || has(s.history(t), "grab.sent") {
		t.Fatal("shadow mode sent a grab")
	}
}
