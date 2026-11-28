package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/testkit"
)

// One running stack serves both kinds: a movie and a show share the
// downloader, the queue and the importer, in either order or at once.

func (s *stack) addShow(t *testing.T) {
	t.Helper()
	if code, body := s.call(t, "POST", "/api/v1/series", `{"tmdbId":77,"monitor":"all","searchNow":true}`); code != 201 {
		t.Fatalf("add series: %d %s", code, body)
	}
}

func (s *stack) movieImported() bool {
	_, err := os.Stat(filepath.Join(s.library, "Example Movie (2021)", "Example Movie (2021) [WEBDL-1080p] - GRP.mkv"))
	return err == nil
}

func (s *stack) showImported() bool {
	season := filepath.Join(s.shows, "Example Show (2020)", "Season 01")
	for _, name := range []string{"Example Show - S01E01 - Pilot [WEBDL-1080p].mkv", "Example Show - S01E02 - Second [WEBDL-1080p].mkv"} {
		if _, err := os.Stat(filepath.Join(season, name)); err != nil {
			return false
		}
	}
	return true
}

// downloading reports whether the queue shows the release past 0%.
func (s *stack) downloading(t *testing.T, release string) bool {
	_, body := s.call(t, "GET", "/api/v1/queue", "")
	var q struct {
		Items []struct {
			Title    string  `json:"title"`
			State    string  `json:"state"`
			Progress float64 `json:"progress"`
		} `json:"items"`
	}
	_ = json.Unmarshal([]byte(body), &q)
	for _, it := range q.Items {
		if it.Title == release && it.State == "downloading" && it.Progress > 0 {
			return true
		}
	}
	return false
}

func TestMovieThenShowInOneStack(t *testing.T) {
	s := start(t, []string{"movie.mkv"})
	s.addMovie(t)
	testkit.Eventually(t, 20*time.Second, "movie imported", s.movieImported)
	s.qb.setHold(true)
	s.addShow(t)
	testkit.Eventually(t, 10*time.Second, "show grabbed", func() bool { return s.qb.added(showHash) })
	testkit.Eventually(t, 10*time.Second, "show progresses in the queue", func() bool { return s.downloading(t, showPack) })
	s.qb.setHold(false)
	testkit.Eventually(t, 20*time.Second, "show imported", s.showImported)
	s.twoImports(t)
}

func TestShowThenMovieInOneStack(t *testing.T) {
	s := start(t, []string{"movie.mkv"})
	s.addShow(t)
	testkit.Eventually(t, 20*time.Second, "show imported", s.showImported)
	s.addMovie(t)
	testkit.Eventually(t, 10*time.Second, "movie grabbed", func() bool { return s.qb.added(infoHash) })
	testkit.Eventually(t, 20*time.Second, "movie imported", s.movieImported)
	s.twoImports(t)
}

func TestMovieAndShowInFlightTogether(t *testing.T) {
	s := start(t, []string{"movie.mkv"})
	s.qb.setHold(true)
	s.addMovie(t)
	s.addShow(t)
	testkit.Eventually(t, 10*time.Second, "both progress in the queue", func() bool {
		return s.downloading(t, releaseTitle) && s.downloading(t, showPack)
	})
	s.qb.setHold(false)
	testkit.Eventually(t, 30*time.Second, "movie imported", s.movieImported)
	testkit.Eventually(t, 30*time.Second, "show imported", s.showImported)
	s.twoImports(t)
}

// twoImports waits for one import.completed per download in History.
func (s *stack) twoImports(t *testing.T) {
	t.Helper()
	testkit.Eventually(t, 5*time.Second, "two imports in history", func() bool { return count(s.history(t), "import.completed") == 2 })
}
