package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/testkit"
)

// The compatibility API driven the way a request manager drives the
// incumbent apps: status → profiles → root folders → tag → lookup → add →
// command → queue, then the library scan sees the import.

// getJSON calls a compatibility route and decodes its body into v.
func (s *stack) getJSON(t *testing.T, method, path, body string, want int, v any) {
	t.Helper()
	code, out := s.call(t, method, path, body)
	if code != want {
		t.Fatalf("%s %s: %d %s", method, path, code, out)
	}
	if v != nil {
		if err := json.Unmarshal([]byte(out), v); err != nil {
			t.Fatalf("%s %s: %v in %s", method, path, err, out)
		}
	}
}

type idName struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Label string `json:"label"`
}

// setup walks the client's connection test and the add dialog's lists.
func (s *stack) setup(t *testing.T, base, root, major string) (profile, tag int64) {
	t.Helper()
	var st struct {
		Version string `json:"version"`
	}
	s.getJSON(t, "GET", base+"/system/status", "", 200, &st)
	if !strings.HasPrefix(st.Version, major+".") {
		t.Fatalf("version %q", st.Version)
	}
	var profiles, roots []idName
	s.getJSON(t, "GET", base+"/qualityprofile", "", 200, &profiles)
	for _, p := range profiles {
		if p.Name == "HD-1080p" {
			profile = p.ID
		}
	}
	s.getJSON(t, "GET", base+"/rootfolder", "", 200, &roots)
	if profile == 0 || len(roots) == 0 || roots[0].Path != root || roots[0].ID == 0 {
		t.Fatalf("profiles %+v roots %+v", profiles, roots)
	}
	var created idName
	s.getJSON(t, "POST", base+"/tag", `{"label":"1 - alice"}`, 201, &created)
	var tags []idName
	s.getJSON(t, "GET", base+"/tag", "", 200, &tags)
	if created.ID == 0 || len(tags) != 1 || tags[0] != created {
		t.Fatalf("tags %+v %+v", created, tags)
	}
	return profile, created.ID
}

type queuePage struct {
	TotalRecords int `json:"totalRecords"`
	Records      []struct {
		SeriesID   int64   `json:"seriesId"`
		EpisodeID  int64   `json:"episodeId"`
		MovieID    int64   `json:"movieId"`
		Size       float64 `json:"size"`
		Sizeleft   float64 `json:"sizeleft"`
		Status     string  `json:"status"`
		Title      string  `json:"title"`
		DownloadID string  `json:"downloadId"`
	} `json:"records"`
}

func (s *stack) waitQueue(t *testing.T, path string) queuePage {
	t.Helper()
	var q queuePage
	testkit.Eventually(t, 15*time.Second, "a queue record", func() bool {
		s.getJSON(t, "GET", path, "", 200, &q)
		return q.TotalRecords > 0
	})
	return q
}

func (s *stack) waitCommand(t *testing.T, base string, id int64) {
	t.Helper()
	testkit.Eventually(t, 15*time.Second, "command completed", func() bool {
		var c struct {
			Status string `json:"status"`
		}
		s.getJSON(t, "GET", fmt.Sprintf("%s/command/%d", base, id), "", 200, &c)
		return c.Status == "completed"
	})
}

func TestSonarrCompatibleRequestFlow(t *testing.T) {
	s := start(t, nil)
	s.qb.setHold(true)
	const base = "/legacy/sonarr/api/v3"
	profile, tag := s.setup(t, base, s.shows, "4")
	s.getJSON(t, "GET", base+"/languageprofile", "", 200, nil)

	var found []struct {
		ID      int64  `json:"id"`
		TvdbID  int64  `json:"tvdbId"`
		Title   string `json:"title"`
		Seasons []struct {
			SeasonNumber int `json:"seasonNumber"`
		} `json:"seasons"`
	}
	s.getJSON(t, "GET", base+"/series/lookup?term=tvdb:321", "", 200, &found)
	if len(found) != 1 || found[0].ID != 0 || found[0].TvdbID != 321 || found[0].Title != showTitle || len(found[0].Seasons) != 1 {
		t.Fatalf("lookup %+v", found)
	}
	add := fmt.Sprintf(`{"tvdbId":321,"title":%q,"qualityProfileId":%d,"languageProfileId":1,"seasons":[{"seasonNumber":1,"monitored":true}],
		"tags":[%d],"seasonFolder":true,"monitored":true,"rootFolderPath":%q,"seriesType":"standard",
		"addOptions":{"ignoreEpisodesWithFiles":true,"searchForMissingEpisodes":true}}`, showTitle, profile, tag, s.shows)
	var added struct {
		ID               int64   `json:"id"`
		QualityProfileID int64   `json:"qualityProfileId"`
		Tags             []int64 `json:"tags"`
		Seasons          []struct {
			SeasonNumber int  `json:"seasonNumber"`
			Monitored    bool `json:"monitored"`
		} `json:"seasons"`
	}
	s.getJSON(t, "POST", base+"/series", add, 201, &added)
	if added.ID == 0 || added.QualityProfileID != profile || len(added.Tags) != 1 || added.Tags[0] != tag ||
		len(added.Seasons) != 1 || !added.Seasons[0].Monitored {
		t.Fatalf("added %+v", added)
	}
	var cmd struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	s.getJSON(t, "POST", base+"/command", fmt.Sprintf(`{"name":"SeriesSearch","seriesId":%d}`, added.ID), 201, &cmd)
	if cmd.ID == 0 || cmd.Name != "SeriesSearch" {
		t.Fatalf("command %+v", cmd)
	}
	s.waitCommand(t, base, cmd.ID)

	q := s.waitQueue(t, base+"/queue?page=1&pageSize=20&includeEpisode=true&includeSeries=true")
	if q.TotalRecords != 2 || q.Records[0].SeriesID != added.ID || q.Records[0].EpisodeID == 0 || q.Records[0].Size <= 0 ||
		q.Records[0].DownloadID != showHash || q.Records[0].Title != showPack {
		t.Fatalf("queue %+v", q)
	}
	var details []json.RawMessage
	s.getJSON(t, "GET", base+"/queue/details", "", 200, &details)
	if len(details) != 2 {
		t.Fatalf("queue details %d", len(details))
	}

	s.qb.setHold(false)
	testkit.Eventually(t, 20*time.Second, "the library scan sees both files", func() bool {
		var list []struct {
			TvdbID     int64 `json:"tvdbId"`
			Statistics struct {
				EpisodeFileCount int `json:"episodeFileCount"`
			} `json:"statistics"`
		}
		s.getJSON(t, "GET", base+"/series?tvdbId=321", "", 200, &list)
		return len(list) == 1 && list[0].Statistics.EpisodeFileCount == 2
	})
	s.getJSON(t, "GET", base+"/series/lookup?term=tvdb:321", "", 200, &found)
	if found[0].ID != added.ID {
		t.Fatalf("lookup after add carries the library id: %+v", found)
	}
}

func TestRadarrCompatibleRequestFlow(t *testing.T) {
	s := start(t, []string{"movie.mkv"})
	s.qb.setHold(true)
	const base = "/legacy/radarr/api/v3"
	profile, tag := s.setup(t, base, s.library, "5")

	var found []struct {
		ID      int64 `json:"id"`
		TmdbID  int64 `json:"tmdbId"`
		HasFile bool  `json:"hasFile"`
	}
	s.getJSON(t, "GET", base+"/movie/lookup?term=tmdb:550", "", 200, &found)
	if len(found) != 1 || found[0].ID != 0 || found[0].TmdbID != 550 {
		t.Fatalf("lookup %+v", found)
	}
	add := fmt.Sprintf(`{"title":%q,"qualityProfileId":%d,"profileId":%d,"titleSlug":"example-movie-550","minimumAvailability":"released",
		"tmdbId":550,"year":2021,"rootFolderPath":%q,"monitored":true,"tags":[%d],"addOptions":{"searchForMovie":true}}`,
		movieTitle, profile, profile, s.library, tag)
	var added struct {
		ID        int64 `json:"id"`
		Monitored bool  `json:"monitored"`
	}
	s.getJSON(t, "POST", base+"/movie", add, 201, &added)
	if added.ID == 0 || !added.Monitored {
		t.Fatalf("added %+v", added)
	}
	var cmd struct {
		ID int64 `json:"id"`
	}
	s.getJSON(t, "POST", base+"/command", fmt.Sprintf(`{"name":"MoviesSearch","movieIds":[%d]}`, added.ID), 201, &cmd)
	s.waitCommand(t, base, cmd.ID)
	q := s.waitQueue(t, base+"/queue?page=1&pageSize=20&includeMovie=true")
	if q.TotalRecords != 1 || q.Records[0].MovieID != added.ID || q.Records[0].DownloadID != infoHash || q.Records[0].Sizeleft < 0 {
		t.Fatalf("queue %+v", q)
	}
	var st struct {
		TotalCount int `json:"totalCount"`
	}
	s.getJSON(t, "GET", base+"/queue/status", "", 200, &st)
	if st.TotalCount != 1 {
		t.Fatalf("queue status %+v", st)
	}

	s.qb.setHold(false)
	testkit.Eventually(t, 20*time.Second, "the library scan sees the file", func() bool {
		var list []struct {
			HasFile bool `json:"hasFile"`
		}
		s.getJSON(t, "GET", base+"/movie?tmdbId=550", "", 200, &list)
		return len(list) == 1 && list[0].HasFile
	})
	var one struct {
		ID      int64 `json:"id"`
		HasFile bool  `json:"hasFile"`
	}
	s.getJSON(t, "GET", base+"/movie/lookup/tmdb?tmdbId=550", "", 200, &one)
	if one.ID != added.ID || !one.HasFile {
		t.Fatalf("lookup after import %+v", one)
	}
}

// Dashboard widgets read counts from all four apps.
func TestDashboardReadsFromEveryCompatibilityAPI(t *testing.T) {
	s := start(t, []string{"movie.mkv"})
	s.addMovie(t)
	testkit.Eventually(t, 20*time.Second, "import.completed in history", func() bool { return has(s.history(t), "import.completed") })

	var movies []struct {
		HasFile     bool `json:"hasFile"`
		Monitored   bool `json:"monitored"`
		IsAvailable bool `json:"isAvailable"`
	}
	s.getJSON(t, "GET", "/legacy/radarr/api/v3/movie", "", 200, &movies)
	if len(movies) != 1 || !movies[0].HasFile || !movies[0].IsAvailable {
		t.Fatalf("movies %+v", movies)
	}
	var paged struct {
		TotalRecords int `json:"totalRecords"`
	}
	s.getJSON(t, "GET", "/legacy/sonarr/api/v3/wanted/missing?pageSize=1", "", 200, &paged)
	s.getJSON(t, "GET", "/legacy/sonarr/api/v3/queue", "", 200, &paged)
	s.getJSON(t, "GET", "/legacy/sonarr/api/v3/series", "", 200, nil)
	s.getJSON(t, "GET", "/legacy/sonarr/api/v3/calendar?start=2020-01-01&end=2020-02-01", "", 200, nil)
	s.getJSON(t, "GET", "/legacy/lidarr/api/v1/artist", "", 200, nil)
	s.getJSON(t, "GET", "/legacy/lidarr/api/v1/wanted/missing", "", 200, &paged)
	s.getJSON(t, "GET", "/legacy/lidarr/api/v1/queue/status", "", 200, nil)

	var indexers []struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Enable bool   `json:"enable"`
	}
	s.getJSON(t, "GET", "/legacy/prowlarr/api/v1/indexer", "", 200, &indexers)
	if len(indexers) != 1 || indexers[0].ID == 0 || !indexers[0].Enable {
		t.Fatalf("indexers %+v", indexers)
	}
	var stats struct {
		Indexers []struct {
			IndexerID       int64 `json:"indexerId"`
			NumberOfQueries int   `json:"numberOfQueries"`
			NumberOfGrabs   int   `json:"numberOfGrabs"`
		} `json:"indexers"`
	}
	s.getJSON(t, "GET", "/legacy/prowlarr/api/v1/indexerstats", "", 200, &stats)
	if len(stats.Indexers) != 1 || stats.Indexers[0].IndexerID != indexers[0].ID || stats.Indexers[0].NumberOfQueries < 1 ||
		stats.Indexers[0].NumberOfGrabs != 1 {
		t.Fatalf("stats %+v", stats)
	}

	req, _ := http.NewRequest("GET", s.api.URL+"/legacy/prowlarr/api/v1/indexer", nil)
	req.Header.Set("X-Api-Key", "wrong")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong key: %d", resp.StatusCode)
	}
}
