package tvauto

import (
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/testkit"
)

func TestHTTPEndpoints(t *testing.T) {
	e := newEnv(t)
	e.add(t, false)
	good := "Example.Show.S02E01.1080p.WEB-DL.x264-GRP"
	e.ix.byEp["Example Show S02E01"] = []string{good, "Other.Show.S02E01.1080p.WEB-DL.x264-GRP"}
	srv := testkit.Serve(t, func(m testkit.Mux) { e.svc.Mount(m) })
	steps := []struct {
		method, path, body string
		code               int
		has                string
	}{
		{"GET", "/api/v1/wanted/missing/episodes?limit=2", "", 200, `"nextCursor":"2"`},
		{"GET", "/api/v1/wanted/missing/episodes?limit=0", "", 400, ""},
		{"GET", "/api/v1/episodes/5/releases", "", 200, `"summary":{`},
		{"GET", "/api/v1/episodes/99/releases", "", 404, ""},
		{"GET", "/api/v1/episodes/x/releases", "", 400, ""},
		{"GET", "/api/v1/series/1/seasons/2/releases", "", 200, `"done":true`},
		{"GET", "/api/v1/series/1/seasons/7/releases", "", 404, ""},
		{"GET", "/api/v1/series/1/seasons/x/releases", "", 400, ""},
		{"GET", "/api/v1/series/x/seasons/1/releases", "", 400, ""},
		{"POST", "/api/v1/series/1/grabs", `{"releaseId":"nope"}`, 404, `"field":"releaseId"`},
		{"POST", "/api/v1/series/1/grabs", `{"releaseId":"Other.Show.S02E01.1080p.WEB-DL.x264-GRP","episodeId":5}`, 409, "confirm_required"},
		{"POST", "/api/v1/series/1/grabs", `{"releaseId":"` + good + `","season":2}`, 201, `"grab":`},
		{"POST", "/api/v1/series/9/grabs", `{"releaseId":"x"}`, 404, ""},
		{"POST", "/api/v1/series/x/grabs", `{}`, 400, ""},
		{"POST", "/api/v1/episodes/5/search", "", 200, `"searches":1`},
		{"POST", "/api/v1/episodes/99/search", "", 404, ""},
		{"POST", "/api/v1/episodes/x/search", "", 400, ""},
		{"POST", "/api/v1/series/1/seasons/2/search", "", 200, `"searches":`},
		{"POST", "/api/v1/series/1/seasons/x/search", "", 400, ""},
		{"POST", "/api/v1/series/x/seasons/1/search", "", 400, ""},
		{"POST", "/api/v1/series/1/search", "", 200, `"searches":`},
		{"POST", "/api/v1/series/x/search", "", 400, ""},
	}
	for _, s := range steps {
		code, body := testkit.Do(t, srv, s.method, s.path, s.body)
		if code != s.code || !strings.Contains(body, s.has) {
			t.Errorf("%s %s: %d %s", s.method, s.path, code, body)
		}
	}
	e.mode = domain.ModeShadow
	if code, body := testkit.Do(t, srv, "POST", "/api/v1/series/1/grabs", `{"releaseId":"Example.Show.S02E01.1080p.WEB-DL.x264-GRP"}`); code != 409 && code != 201 {
		t.Errorf("shadow grab: %d %s", code, body)
	}
	e.mode = domain.ModeDisabled
	for _, p := range []string{"/api/v1/episodes/5/releases", "/api/v1/series/1/seasons/2/releases"} {
		if code, body := testkit.Do(t, srv, "GET", p, ""); code != 409 || !strings.Contains(body, "kind_mode") {
			t.Errorf("disabled %s: %d %s", p, code, body)
		}
	}
	e.q.err = errFake
	e.mode = domain.ModeActive
	if code, _ := testkit.Do(t, srv, "POST", "/api/v1/series/1/grabs", `{"releaseId":"`+good+`","confirm":true}`); code != 502 {
		t.Errorf("queue failure: %d", code)
	}
}
