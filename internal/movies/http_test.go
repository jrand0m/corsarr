package movies

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/testkit"
)

func TestHTTPEndpoints(t *testing.T) {
	e := newEnv(t)
	srv := testkit.Serve(t, func(m testkit.Mux) { e.svc.Mount(m) })
	steps := []struct {
		method, path, body string
		code               int
		has                string
	}{
		{"POST", "/api/v1/movies", `{"tmdbId":603,"searchNow":true}`, 201, `"title":"The Matrix"`},
		{"POST", "/api/v1/movies", `{"tmdbId":603}`, 200, `"id":1`},
		{"POST", "/api/v1/movies", `{"tmdbId":0}`, 400, `"field":"tmdbId"`},
		{"POST", "/api/v1/movies", `{"bogus":1}`, 400, "invalid JSON"},
		{"GET", "/api/v1/movies?limit=1", "", 200, `"nextCursor":"1"`},
		{"GET", "/api/v1/movies?limit=0", "", 400, ""},
		{"GET", "/api/v1/movies/lookup?term=matrix", "", 200, `"The Matrix"`},
		{"GET", "/api/v1/movies/lookup", "", 400, `"field":"term"`},
		{"GET", "/api/v1/movies/1", "", 200, `"profileKey":"hd1080p"`},
		{"GET", "/api/v1/movies/9", "", 404, "not_found"},
		{"GET", "/api/v1/movies/x", "", 400, ""},
		{"PUT", "/api/v1/movies/1", `{"monitored":false}`, 200, `"monitored":false`},
		{"PUT", "/api/v1/movies/9", `{}`, 404, ""},
		{"PUT", "/api/v1/movies/x", `{}`, 400, ""},
		{"GET", "/api/v1/movies/1/move?rootFolder=", "", 200, `"files":[]`},
		{"GET", "/api/v1/movies/1/move?rootFolder=/not/a/root", "", 400, `"field":"rootFolder"`},
		{"GET", "/api/v1/movies/9/move", "", 404, ""},
		{"GET", "/api/v1/movies/x/move", "", 400, ""},
		{"DELETE", "/api/v1/movies/1", "", 204, ""},
		{"DELETE", "/api/v1/movies/1", "", 404, ""},
		{"DELETE", "/api/v1/movies/x", "", 400, ""},
	}
	for _, s := range steps {
		code, body := testkit.Do(t, srv, s.method, s.path, s.body)
		if code != s.code || !strings.Contains(body, s.has) {
			t.Errorf("%s %s: %d %s", s.method, s.path, code, body)
		}
	}
	e.meta = nil
	if code, _ := testkit.Do(t, srv, "GET", "/api/v1/movies/lookup?term=zzz", ""); code != 503 {
		t.Errorf("no metadata: %d", code)
	}
}

func TestAnswersPointArtworkAtTheCache(t *testing.T) {
	e := newEnv(t)
	e.svc.d.Art = func(r domain.ArtRole, o string) string { return "cache/" + string(r) + "/" + o }
	srv := testkit.Serve(t, func(m testkit.Mux) { e.svc.Mount(m) })
	poster, fanart := `"posterUrl":"cache/p/https://img.example/603.jpg"`, `"fanartUrl":"cache/b/https://img.example/603b.jpg"`
	for _, s := range []struct{ method, path, body string }{
		{"POST", "/api/v1/movies", `{"tmdbId":603}`}, {"GET", "/api/v1/movies", ""}, {"GET", "/api/v1/movies/1", ""},
		{"PUT", "/api/v1/movies/1", `{"monitored":false}`}, {"GET", "/api/v1/movies/lookup?term=tmdb:603", ""},
	} {
		_, body := testkit.Do(t, srv, s.method, s.path, s.body)
		if !strings.Contains(body, poster) || !strings.Contains(body, fanart) {
			t.Errorf("%s %s: %s", s.method, s.path, body)
		}
	}
	if m, _ := e.svc.Get(ctx, 1); m.PosterURL != "https://img.example/603.jpg" {
		t.Fatal("stored address must stay the provider's", m.PosterURL)
	}
}

func TestWriteErrUpstream(t *testing.T) {
	w := httptest.NewRecorder()
	WriteErr(w, errors.New("tmdb down"))
	if w.Code != http.StatusBadGateway {
		t.Fatal(w.Code)
	}
}
