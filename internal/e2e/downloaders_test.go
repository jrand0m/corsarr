package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/testkit"
)

func TestDownloaderControlThroughTheAPI(t *testing.T) {
	s := start(t, []string{"movie.mkv"})
	s.qb.setHold(true) // keep the torrent downloading
	s.addMovie(t)
	testkit.Eventually(t, 10*time.Second, "grab.sent in history", func() bool { return has(s.history(t), "grab.sent") })
	for _, c := range []struct{ method, path, body, has string }{
		{"GET", "/api/v1/downloaders", "", `"type":"qbittorrent"`},
		{"GET", "/api/v1/downloaders/qbit", "", `"freeSpace":1073741824`},
		{"GET", "/api/v1/queue?downloader=qbit&kind=movie", "", `"downloadId":"`},
		{"GET", "/api/v1/queue/1", "", `"trackers":[{"url":"https://tracker.example"`},
		{"POST", "/api/v1/queue/1/recheck", "", `"id":1`},
		{"POST", "/api/v1/queue/1/reannounce", "", `"id":1`},
		{"POST", "/api/v1/downloaders/qbit/pause-all", "", `"key":"qbit"`},
		{"POST", "/api/v1/downloaders/qbit/speed-limits", `{"downKiB":100}`, `"key":"qbit"`},
		{"POST", "/api/v1/downloaders/qbit/test", "", `"status":"ok"`},
	} {
		if code, body := s.call(t, c.method, c.path, c.body); code != 200 || !strings.Contains(body, c.has) {
			t.Errorf("%s %s: %d %s", c.method, c.path, code, body)
		}
	}
	if _, body := s.call(t, "GET", "/api/v1/queue/1", ""); strings.Contains(body, "/announce/key") {
		t.Fatal("tracker passkey leaked")
	}
}

func TestShadowModeRecordsDownloaderWrites(t *testing.T) {
	s := startIn(t, domain.ModeShadow, []string{"movie.mkv"})
	if code, body := s.call(t, "POST", "/api/v1/downloaders/qbit/pause-all", ""); code != 409 || !strings.Contains(body, "shadowed") {
		t.Fatalf("pause-all in shadow: %d %s", code, body)
	}
	testkit.Eventually(t, 5*time.Second, "downloader.shadowed in history", func() bool { return has(s.history(t), "downloader.shadowed") })
	if code, body := s.call(t, "GET", "/api/v1/downloaders", ""); code != 200 || !strings.Contains(body, `"stats"`) {
		t.Fatalf("reads work in shadow: %d %s", code, body)
	}
}
