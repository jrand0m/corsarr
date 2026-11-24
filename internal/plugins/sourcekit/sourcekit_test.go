package sourcekit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
)

// fakeService is a source protocol server with one album of two files.
type fakeService struct {
	srv  *httptest.Server
	mu   sync.Mutex
	auth []string
	fail bool
}

func newService(t *testing.T) *fakeService {
	f := &fakeService{}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeService) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.auth = append(f.auth, r.Header.Get("Authorization")+r.Header.Get("Cookie"))
	fail := f.fail
	f.mu.Unlock()
	switch {
	case fail:
		w.WriteHeader(http.StatusUnauthorized)
	case r.URL.Path == "/search":
		_, _ = w.Write([]byte(`{"items":[{"id":"alb1","artist":"Example Artist","album":"First Album","year":2020,"format":"flac",
			"bitDepth":24,"tracks":["01 Opening.flac","02 Closing.flac"],"size":4096,"url":"https://site.invalid/a"}]}`))
	case r.URL.Path == "/items/alb1/files":
		_, _ = w.Write([]byte(`{"files":[{"name":"01 Opening.flac","url":"` + f.srv.URL + `/f/1","size":3},
			{"name":"../02 Closing.flac","url":"` + f.srv.URL + `/f/2","size":3}]}`))
	case r.URL.Path == "/items/empty/files":
		_, _ = w.Write([]byte(`{"files":[]}`))
	case strings.HasPrefix(r.URL.Path, "/f/"):
		_, _ = w.Write([]byte("abc"))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newSource(t *testing.T, f *fakeService) *Source {
	t.Helper()
	site := HTTPSite{Base: f.srv.URL, Token: "tok", Format: "flac", HTTP: http.DefaultClient, Name: "Example"}
	s, err := New(plugin.Instance{Key: "camp", Name: "Camp", PluginID: "x"}, site, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func musicReq() domain.SearchRequest {
	return domain.SearchRequest{Kind: domain.KindMusic, Mode: domain.SearchMusic, Text: "Example Artist First Album",
		IDs: map[string]string{"artist": "Example Artist", "album": "First Album"}}
}

func TestSourceIsAnIndexerAndADownloader(t *testing.T) {
	f := newService(t)
	s := newSource(t, f)
	if caps, err := s.Caps(context.Background()); err != nil || caps.Modes[0] != domain.SearchMusic {
		t.Fatalf("caps %v %v", caps, err)
	}
	if rels, cur, err := s.Feed(context.Background(), domain.KindMusic, "c"); rels != nil || cur != "c" || err != nil {
		t.Fatal("a source has no feed")
	}
	rels, _ := s.Search(context.Background(), musicReq())
	r := rels[0]
	if r.Title != "Example Artist - First Album (2020) [FLAC 24bit]" || r.Protocol != domain.ProtoSource || r.ID != "camp:alb1" ||
		len(r.FileList) != 2 || !s.Owns(r) || s.CategoryFor(domain.KindMusic) != "camp" {
		t.Fatalf("%+v", r)
	}
	if f.auth[0] != "Bearer tok" {
		t.Fatalf("auth %q", f.auth[0])
	}
	if rels, _ := s.Search(context.Background(), domain.SearchRequest{Kind: domain.KindMovie}); rels != nil {
		t.Fatal("movies are not searched on a source")
	}
	p, _ := s.Resolve(context.Background(), r)
	conformance.Downloader(t, s, plugin.AddRequest{Payload: p, Category: "camp"})
}

func waitState(t *testing.T, s *Source, id string) plugin.Snapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		list, _ := s.List(context.Background(), nil)
		for _, sn := range list {
			if sn.DownloadID == id && sn.State != domain.StateDownloading {
				return sn
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("fetch did not finish")
	return plugin.Snapshot{}
}

func TestAddFetchesEveryFileIntoTheAlbumFolder(t *testing.T) {
	f := newService(t)
	s := newSource(t, f)
	id, err := s.Add(context.Background(), plugin.AddRequest{Payload: plugin.Payload{Kind: "source", URL: "alb1", Name: "A - B/C"}, Category: "camp"})
	if err != nil {
		t.Fatal(err)
	}
	sn := waitState(t, s, id)
	if sn.State != domain.StateCompleted || sn.Progress != 1 || sn.Size != 6 || filepath.Base(sn.ContentPath) != "A - B-C" {
		t.Fatalf("%+v", sn)
	}
	for _, name := range []string{"01 Opening.flac", ".-02 Closing.flac"} {
		if b, err := os.ReadFile(filepath.Join(sn.ContentPath, name)); err != nil || string(b) != "abc" {
			t.Fatalf("%s: %q %v", name, b, err)
		}
	}
	if list, _ := s.List(context.Background(), []string{"other"}); len(list) != 0 {
		t.Fatal("category filter")
	}
	if err := s.Remove(context.Background(), id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sn.ContentPath); !os.IsNotExist(err) {
		t.Fatal("folder removed")
	}
}

func TestFailuresAreReported(t *testing.T) {
	f := newService(t)
	s := newSource(t, f)
	id, _ := s.Add(context.Background(), plugin.AddRequest{Payload: plugin.Payload{Kind: "source", URL: "empty", Name: "x"}})
	if sn := waitState(t, s, id); sn.State != domain.StateFailed || sn.Error == "" {
		t.Fatalf("%+v", sn)
	}
	if _, err := s.Add(context.Background(), plugin.AddRequest{Payload: plugin.Payload{Kind: "torrent", URL: "m"}}); err == nil {
		t.Fatal("a source fetches only its own releases")
	}
	if _, err := s.Resolve(context.Background(), domain.Release{}); err == nil {
		t.Fatal("no item")
	}
	f.fail = true
	if err := s.Test(context.Background()); plugin.ClassOf(err) != plugin.ClassAuth {
		t.Fatalf("auth failure: %v", err)
	}
	if _, err := New(plugin.Instance{PluginID: "x"}, HTTPSite{}, ""); err == nil {
		t.Fatal("download dir required")
	}
	if _, err := ParseBase("x", "ftp://y"); err == nil {
		t.Fatal("bad base")
	}
	if Title(Item{Artist: "A", Album: "B", Format: "mp3", Bitrate: 320}) != "A - B [MP3 320kbps]" || safeName("..") != "." && safeName("") != "album" {
		t.Fatal("names")
	}
}

func TestFileOnAnotherHostGetsNoCredential(t *testing.T) {
	f := newService(t)
	other := newService(t)
	site := HTTPSite{Base: f.srv.URL, Cookie: "identity=x", HTTP: http.DefaultClient, Name: "Example"}
	body, err := site.Fetch(context.Background(), FileRef{URL: other.srv.URL + "/f/1"})
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	if other.auth[0] != "" {
		t.Fatalf("credential leaked to another host: %q", other.auth[0])
	}
	if _, err := site.Fetch(context.Background(), FileRef{URL: "file:///etc/passwd"}); err == nil {
		t.Fatal("non-http file address")
	}
	if _, err := site.Fetch(context.Background(), FileRef{URL: f.srv.URL + "/missing"}); err == nil {
		t.Fatal("404 file")
	}
}
