package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/testkit"
)

// The TV pipeline: add a series with search → season pack search by tvdb id
// → one grab linked to both episodes with category tv → completion →
// hardlink import of each episode into its season folder → Plex refresh of
// the season folder → one ntfy message naming the show once → History.
func TestTVPipelineEndToEnd(t *testing.T) {
	s := start(t, nil)
	if code, body := s.call(t, "GET", "/api/v1/series/lookup?term=example", ""); code != 200 || !strings.Contains(body, `"libraryId":0`) {
		t.Fatalf("lookup: %d %s", code, body)
	}
	if code, body := s.call(t, "POST", "/api/v1/series", `{"tmdbId":77,"monitor":"all","searchNow":true}`); code != 201 {
		t.Fatalf("add series: %d %s", code, body)
	}
	testkit.Eventually(t, 20*time.Second, "import.completed in history", func() bool { return has(s.history(t), "import.completed") })

	season := filepath.Join(s.shows, "Example Show (2020)", "Season 01")
	for n, name := range map[string]string{
		"Example.Show.S01E01.1080p.WEB-DL.x264-GRP.mkv": "Example Show - S01E01 - Pilot [WEBDL-1080p].mkv",
		"Example.Show.S01E02.1080p.WEB-DL.x264-GRP.mkv": "Example Show - S01E02 - Second [WEBDL-1080p].mkv",
	} {
		src, err1 := os.Stat(filepath.Join(s.dl, showPack, n))
		dst, err2 := os.Stat(filepath.Join(season, name))
		if err1 != nil || err2 != nil || !os.SameFile(src, dst) {
			t.Fatalf("hardlink %s: %v %v", name, err1, err2)
		}
	}
	if !s.qb.added(showHash) {
		t.Fatal("the season pack was not sent to the downloader")
	}
	for _, q := range s.tz.tvQueries {
		if strings.Contains(q, "ep=") {
			t.Fatalf("a grabbed pack must not be followed by episode searches: %v", s.tz.tvQueries)
		}
	}
	testkit.Eventually(t, 5*time.Second, "Plex refresh of the season folder", func() bool { return has(s.plex.refreshed(), season) })
	testkit.Eventually(t, 5*time.Second, "one ntfy message", func() bool { return len(s.ntfy.messages("/corsarr")) >= 1 })
	time.Sleep(300 * time.Millisecond)
	if msgs := s.ntfy.messages("/corsarr"); len(msgs) != 1 || msgs[0] != "Imported: Example Show · S01 · 2 episodes" {
		t.Fatalf("ntfy got %v", msgs)
	}
	hist := s.history(t)
	for _, want := range []string{"item.added", "grab.sent", "import.completed"} {
		if !has(hist, want) {
			t.Errorf("history lacks %s: %v", want, hist)
		}
	}
	if n := count(hist, "grab.sent"); n != 1 {
		t.Fatalf("one grab for the pack, got %d", n)
	}
	if _, body := s.call(t, "GET", "/api/v1/series/1", ""); strings.Count(body, `"status":"downloaded"`) != 2 {
		t.Fatalf("both episodes have their file: %s", body)
	}
	if _, body := s.call(t, "GET", "/api/v1/wanted/missing/episodes", ""); strings.Contains(body, showTitle) {
		t.Fatalf("imported episodes are no longer missing: %s", body)
	}
	if code, body := s.call(t, "GET", "/api/v1/queue", ""); code != 200 || strings.Contains(body, showPack) {
		t.Fatalf("imported download leaves the queue: %s", body)
	}
}

func TestTVShadowModeDecidesButGrabsNothing(t *testing.T) {
	s := startKinds(t, domain.ModeActive, domain.ModeShadow, nil)
	if code, body := s.call(t, "POST", "/api/v1/series", `{"tmdbId":77,"searchNow":true}`); code != 201 {
		t.Fatalf("add series: %d %s", code, body)
	}
	testkit.Eventually(t, 10*time.Second, "decision.shadowed in history", func() bool { return has(s.history(t), "decision.shadowed") })
	time.Sleep(300 * time.Millisecond)
	if s.qb.added(showHash) || has(s.history(t), "grab.sent") {
		t.Fatal("shadow mode sent a grab")
	}
}

func count(list []string, want string) int {
	n := 0
	for _, v := range list {
		if v == want {
			n++
		}
	}
	return n
}
