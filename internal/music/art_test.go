package music

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestArtistCoverIsTheNewestAlbum(t *testing.T) {
	d := func(y int) *time.Time { v := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC); return &v }
	albums := []domain.Album{{MBID: "undated", PrimaryType: domain.AlbumTypeAlbum}, {MBID: "old", PrimaryType: domain.AlbumTypeAlbum, ReleaseDate: d(2001)},
		{MBID: "single", PrimaryType: "single", ReleaseDate: d(2020)}, {MBID: "new", PrimaryType: domain.AlbumTypeAlbum, ReleaseDate: d(2010)}}
	if got := ArtistCover(albums); got != "https://coverartarchive.org/release-group/new/front-500" {
		t.Fatal(got)
	}
	if ArtistCover(albums[2:3]) != "" || CoverURL("") != "" {
		t.Fatal("no album, no cover")
	}
	if got := ArtistCover(albums[:1]); !strings.Contains(got, "/undated/") {
		t.Fatal("an undated album still gives a cover", got)
	}
}

func TestAnswersLinkCoversThroughTheCache(t *testing.T) {
	f := newFixture(t)
	f.svc.d.Art = func(r domain.ArtRole, o string) string { return "cache/" + string(r) + "/" + o }
	if _, _, err := f.svc.Add(context.Background(), AddRequest{MBID: "a1"}); err != nil {
		t.Fatal(err)
	}
	m := mux{http.NewServeMux()}
	f.svc.Mount(m)
	get := func(path string) string {
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Body.String()
	}
	newest := `"coverUrl":"cache/p/https://coverartarchive.org/release-group/g3/front-500"`
	for _, path := range []string{"/api/v1/artists", "/api/v1/artists/1"} {
		if body := get(path); !strings.Contains(body, newest) {
			t.Errorf("%s: %s", path, body)
		}
	}
	if body := get("/api/v1/albums/1"); !strings.Contains(body, `"coverUrl":"cache/p/https://coverartarchive.org/release-group/g1/front-500"`) {
		t.Errorf("album: %s", body)
	}
}
