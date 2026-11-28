package tv

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/testkit"
)

func TestHTTPEndpoints(t *testing.T) {
	h := newHarness(t)
	srv := testkit.Serve(t, func(m testkit.Mux) { h.svc.Mount(m) })
	steps := []struct {
		method, path, body string
		code               int
		has                string
	}{
		{"POST", "/api/v1/series", `{"tmdbId":77,"monitor":"all","searchNow":true}`, 201, `"title":"Example Show"`},
		{"POST", "/api/v1/series", `{"tmdbId":77}`, 200, `"id":1`},
		{"POST", "/api/v1/series", `{"tmdbId":0}`, 400, `"field":"tmdbId"`},
		{"POST", "/api/v1/series", `{"bogus":1}`, 400, "invalid JSON"},
		{"GET", "/api/v1/series?limit=1", "", 200, `"episodeFileCount":0`},
		{"GET", "/api/v1/series?limit=0", "", 400, ""},
		{"GET", "/api/v1/series/lookup?term=example", "", 200, `"libraryId":1`},
		{"GET", "/api/v1/series/lookup", "", 400, `"field":"term"`},
		{"GET", "/api/v1/series/1", "", 200, `"status":"notAired"`},
		{"GET", "/api/v1/series/9", "", 404, "not_found"},
		{"GET", "/api/v1/series/x", "", 400, ""},
		{"PUT", "/api/v1/series/1", `{"profileKey":"any"}`, 200, `"profileKey":"any"`},
		{"PUT", "/api/v1/series/1", `{"profileKey":"x"}`, 400, `"field":"profileKey"`},
		{"PUT", "/api/v1/series/x", `{}`, 400, ""},
		{"PUT", "/api/v1/series/1/seasons/1", `{"monitored":false}`, 200, `"seasons":[`},
		{"PUT", "/api/v1/series/1/seasons/x", `{}`, 400, ""},
		{"PUT", "/api/v1/series/x/seasons/1", `{}`, 400, ""},
		{"PUT", "/api/v1/series/1/seasons/7", `{"monitored":true}`, 404, ""},
		{"PUT", "/api/v1/episodes/monitor", `{"ids":[2,3],"monitored":true}`, 200, `"updated":2`},
		{"PUT", "/api/v1/episodes/monitor", `{"ids":[]}`, 400, `"field":"ids"`},
		{"PUT", "/api/v1/episodes/monitor", `{"ids":[99]}`, 404, ""},
		{"PUT", "/api/v1/episodes/monitor", `{`, 400, ""},
		{"POST", "/api/v1/series/1/refresh", "", 200, `"episodes":[`},
		{"POST", "/api/v1/series/9/refresh", "", 404, ""},
		{"POST", "/api/v1/series/x/refresh", "", 400, ""},
		{"GET", "/api/v1/calendar/tv?start=2024-02-20T00:00:00Z&end=2024-03-10T00:00:00Z", "", 200, `"Return"`},
		{"GET", "/api/v1/calendar/tv", "", 200, `"items":[`},
		{"GET", "/api/v1/calendar/tv?start=yesterday", "", 400, `"field":"start"`},
		{"DELETE", "/api/v1/series/1", "", 204, ""},
		{"DELETE", "/api/v1/series/1", "", 404, ""},
		{"DELETE", "/api/v1/series/x", "", 400, ""},
	}
	for _, s := range steps {
		code, body := testkit.Do(t, srv, s.method, s.path, s.body)
		if code != s.code || !strings.Contains(body, s.has) {
			t.Errorf("%s %s: %d %s", s.method, s.path, code, body)
		}
	}
	h.svc.d.Metadata = func() (plugin.SeriesMetadata, bool) { return nil, false }
	if code, _ := testkit.Do(t, srv, "GET", "/api/v1/series/lookup?term=x", ""); code != 503 {
		t.Errorf("no metadata: %d", code)
	}
	h.mode = domain.ModeDisabled
	if code, body := testkit.Do(t, srv, "GET", "/api/v1/series", ""); code != 409 || !strings.Contains(body, "kind_mode") {
		t.Errorf("disabled: %d %s", code, body)
	}
}

func TestAnswersPointArtworkAtTheCache(t *testing.T) {
	h := newHarness(t)
	h.meta.set(func(r *plugin.SeriesRecord) {
		r.PosterURL, r.FanartURL = "https://img.example/p.jpg", "https://img.example/b.jpg"
	})
	h.svc.d.Art = func(r domain.ArtRole, o string) string { return "cache/" + string(r) + "/" + o }
	srv := testkit.Serve(t, func(m testkit.Mux) { h.svc.Mount(m) })
	poster, fanart := `"posterUrl":"cache/p/https://img.example/p.jpg"`, `"fanartUrl":"cache/b/https://img.example/b.jpg"`
	for _, s := range []struct{ method, path, body string }{
		{"POST", "/api/v1/series", `{"tmdbId":77,"monitor":"all"}`}, {"GET", "/api/v1/series", ""}, {"GET", "/api/v1/series/1", ""},
		{"PUT", "/api/v1/series/1", `{"profileKey":"any"}`}, {"GET", "/api/v1/series/lookup?term=example", ""},
	} {
		_, body := testkit.Do(t, srv, s.method, s.path, s.body)
		if !strings.Contains(body, poster) || !strings.Contains(body, fanart) {
			t.Errorf("%s %s: %s", s.method, s.path, body)
		}
	}
	if _, body := testkit.Do(t, srv, "GET", "/api/v1/calendar/tv?start=2000-01-01T00:00:00Z&end=2100-01-01T00:00:00Z", ""); !strings.Contains(body, poster) {
		t.Errorf("calendar: %s", body)
	}
	if got := WithArt(nil, SeriesRef{PosterURL: "x"}); got.PosterURL != "x" {
		t.Fatal(got)
	}
}

func TestWriteErrUpstream(t *testing.T) {
	w := httptest.NewRecorder()
	WriteErr(w, errors.New("provider down"))
	if w.Code != http.StatusBadGateway {
		t.Fatal(w.Code)
	}
}
