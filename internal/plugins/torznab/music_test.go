package torznab

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestSearchMusicByArtistAlbumAndYear(t *testing.T) {
	var mu sync.Mutex
	var last url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/>` +
				`<audio-search available="yes" supportedParams="q,artist,album,year"/></searching>` +
				`<categories><category id="3000" name="Audio"/></categories></caps>`))
			return
		}
		mu.Lock()
		last = r.URL.Query()
		mu.Unlock()
		_, _ = w.Write([]byte(`<rss><channel></channel></rss>`))
	}))
	defer srv.Close()
	ix := newIndexer(t, srv.URL, nil)
	req := domain.SearchRequest{Kind: domain.KindMusic, Mode: domain.SearchMusic, Text: "Example Artist First Album", Year: 2020,
		IDs: map[string]string{"artist": "Example Artist", "album": "First Album"}}
	if _, err := ix.Search(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	// nothing found by artist and album: the text search runs last
	if last.Get("t") != "search" || last.Get("q") != "Example Artist First Album" {
		t.Fatalf("fallback %v", last)
	}
	q, ok := ix.idQuery(req, mustCaps(t, ix))
	if !ok || q.Get("t") != "music" || q.Get("artist") != "Example Artist" || q.Get("album") != "First Album" || q.Get("year") != "2020" {
		t.Fatalf("music query %v", q)
	}
}

func mustCaps(t *testing.T, ix *Indexer) parsedCaps {
	t.Helper()
	c, err := ix.capsOrLoad(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return c
}
