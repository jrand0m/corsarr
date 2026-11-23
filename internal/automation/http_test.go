package automation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/testkit"
)

func TestHTTPEndpoints(t *testing.T) {
	e := newEnv(t)
	tooSmall := rel("Example.Movie.2021.1080p.WEB-DL.x264-TINY", 1e6, 10)
	exe := rel("Example.Movie.2021.1080p.WEB-DL.x264-BAD", 4e9, 10)
	exe.FileList = []string{"a.exe"}
	e.ix.rels = []domain.Release{rel(good, 4e9, 10), tooSmall, exe}
	srv := testkit.Serve(t, func(m testkit.Mux) { e.svc.Mount(m) })

	code, body := testkit.Do(t, srv, "GET", "/api/v1/movies/1/releases", "")
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if code != 200 || len(lines) != 4 || !strings.Contains(lines[3], `"summary"`) || !strings.Contains(lines[0], `"decision"`) {
		t.Fatalf("ndjson %d:\n%s", code, body)
	}
	var row struct {
		Record Decided `json:"record"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &row); err != nil || row.Record.Release.Title != good || !row.Record.Decision.Accepted {
		t.Fatalf("%+v %v", row, err)
	}
	steps := []struct {
		method, path, body string
		code               int
		has                string
	}{
		{"GET", "/api/v1/movies/9/releases", "", 404, ""},
		{"GET", "/api/v1/movies/x/releases", "", 400, ""},
		{"POST", "/api/v1/grabs", `{"movieId":1,"releaseId":"` + exe.ID + `","confirm":true}`, 403, `"refused"`},
		{"POST", "/api/v1/grabs", `{"movieId":1,"releaseId":"` + tooSmall.ID + `"}`, 409, `"confirm_required"`},
		{"POST", "/api/v1/grabs", `{"movieId":1,"releaseId":"` + tooSmall.ID + `","confirm":true}`, 201, `"grab"`},
		{"POST", "/api/v1/grabs", `{"movieId":1,"releaseId":"nope"}`, 404, ""},
		{"POST", "/api/v1/grabs", `{"movieId":9,"releaseId":"x"}`, 404, ""},
		{"POST", "/api/v1/grabs", `{"x":1}`, 400, ""},
		{"POST", "/api/v1/movies/1/search", "", 200, `"releases":3`},
		{"POST", "/api/v1/movies/9/search", "", 404, ""},
		{"POST", "/api/v1/movies/x/search", "", 400, ""},
		{"GET", "/api/v1/wanted/missing", "", 200, `"reason":"missing"`},
		{"GET", "/api/v1/wanted/missing?limit=0", "", 400, ""},
		{"POST", "/api/v1/blocklist", `{"movieId":1,"release":{"title":"Bad.Release"},"reason":"fake"}`, 201, `"source":"manual"`},
		{"POST", "/api/v1/blocklist", `{"movieId":1,"release":{}}`, 400, ""},
		{"POST", "/api/v1/blocklist", `{"movieId":9,"release":{"title":"x"}}`, 404, ""},
		{"POST", "/api/v1/blocklist", `{"bogus":1}`, 400, ""},
		{"GET", "/api/v1/blocklist", "", 200, "Bad.Release"},
		{"GET", "/api/v1/blocklist?limit=999", "", 400, ""},
		{"DELETE", "/api/v1/blocklist/1", "", 204, ""},
		{"DELETE", "/api/v1/blocklist/1", "", 404, ""},
		{"DELETE", "/api/v1/blocklist/x", "", 400, ""},
	}
	for _, s := range steps {
		code, body := testkit.Do(t, srv, s.method, s.path, s.body)
		if code != s.code || !strings.Contains(body, s.has) {
			t.Errorf("%s %s: %d %s", s.method, s.path, code, body)
		}
	}
	if len(e.rec.Of(domain.EvBlocklistRemoved)) != 1 {
		t.Fatal("blocklist.removed")
	}
	e.q.err = errors.New("downloader down")
	if code, _ := testkit.Do(t, srv, "POST", "/api/v1/grabs", `{"movieId":1,"releaseId":"`+rel(good, 0, 0).ID+`"}`); code != 502 {
		t.Fatalf("grab failure: %d", code)
	}
}
