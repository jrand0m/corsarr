package plex

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
)

type fake struct {
	srv       *httptest.Server
	mu        sync.Mutex
	refreshes []string // "<section>?<path>"
	identity  string
	failSec   string // section whose refresh answers 500
}

func newFake(t *testing.T) *fake {
	t.Helper()
	f := &fake{identity: "identity.json"}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func read(name string) []byte {
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		panic(err)
	}
	return b
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/identity" {
		_, _ = w.Write(read(f.identity))
		return
	}
	if r.Header.Get("X-Plex-Token") != "tok" || r.Header.Get("Accept") != "application/json" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/library/sections":
		_, _ = w.Write(read("sections.json"))
	case strings.HasSuffix(r.URL.Path, "/refresh"):
		sec := strings.Split(r.URL.Path, "/")[3]
		if sec == f.failSec {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		f.mu.Lock()
		f.refreshes = append(f.refreshes, sec+"?"+r.URL.Query().Get("path"))
		f.mu.Unlock()
	default:
		http.NotFound(w, r)
	}
}

func newServer(t *testing.T, base string, extra plugin.Settings) *Server {
	t.Helper()
	s := plugin.Settings{"url": base + "/", "token": "tok"}
	for k, v := range extra {
		s[k] = v
	}
	p, err := New(plugin.Instance{Key: "plex", Settings: s, Env: conformance.Env(http.DefaultClient)})
	if err != nil {
		t.Fatal(err)
	}
	return p.(*Server)
}

func TestConformance(t *testing.T) {
	reg, ok := plugin.Default.Get(ID)
	if !ok {
		t.Fatal("plex not registered")
	}
	conformance.Manifest(t, reg.Manifest, plugin.KindMediaServer)
	if len(reg.Manifest.SetupFlows) != 1 || reg.Manifest.SetupFlows[0] != plugin.FlowPinPairing {
		t.Errorf("pinPairing flow must be declared: %v", reg.Manifest.SetupFlows)
	}
	f := newFake(t)
	conformance.MediaServer(t, newServer(t, f.srv.URL, nil), []string{"/media/movies/Some Movie (2021)"})
}

func TestErrorMapping(t *testing.T) {
	conformance.ErrorMapping(t, func(url string) plugin.Tester { return newServer(t, url, nil) },
		conformance.ErrorCase{Name: "garbage", Want: plugin.ClassParse,
			Handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }},
		conformance.ErrorCase{Name: "no machine id", Want: plugin.ClassParse,
			Handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"MediaContainer":{}}`)) }},
		conformance.ErrorCase{Name: "other 4xx", Want: plugin.ClassServerError,
			Handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }},
	)
}

func TestRevokedTokenIsAuth(t *testing.T) {
	f := newFake(t)
	c := newServer(t, f.srv.URL, plugin.Settings{"token": "revoked"})
	if err := c.Test(t.Context()); plugin.ClassOf(err) != plugin.ClassAuth {
		t.Fatalf("identity needs no token, sections do: %v", err)
	}
}

func TestIdentityMismatch(t *testing.T) {
	f := newFake(t)
	if err := newServer(t, f.srv.URL, plugin.Settings{"machineId": "abc123machine"}).Test(t.Context()); err != nil {
		t.Fatalf("matching identity: %v", err)
	}
	err := newServer(t, f.srv.URL, plugin.Settings{"machineId": "someone-else"}).Test(t.Context())
	if plugin.ClassOf(err) != plugin.ClassIdentityMismatch {
		t.Fatalf("%v", err)
	}
}

func TestRefreshUsesLongestLocation(t *testing.T) {
	f := newFake(t)
	c := newServer(t, f.srv.URL, nil)
	paths := []string{"/media/movies/Some Movie (2021)", "/media/movies/kids/Cartoon (2020)", "/media/kids2/Other", "/media/tv/Show/Season 1"}
	if err := c.RefreshPaths(t.Context(), paths); err != nil {
		t.Fatal(err)
	}
	want := []string{"1?/media/movies/Some Movie (2021)", "2?/media/movies/kids/Cartoon (2020)", "2?/media/kids2/Other", "3?/media/tv/Show/Season 1"}
	if strings.Join(f.refreshes, "|") != strings.Join(want, "|") {
		t.Errorf("refreshes %q", f.refreshes)
	}
}

func TestRefreshDedupesAndHonoursPathBoundary(t *testing.T) {
	f := newFake(t)
	c := newServer(t, f.srv.URL, nil)
	err := c.RefreshPaths(t.Context(), []string{"/media/tv/S/", "/media/tv/S", "/media/tvshows/x"})
	if plugin.ClassOf(err) != plugin.ClassPath || !strings.Contains(err.Error(), "/media/tvshows/x") {
		t.Fatalf("/media/tvshows is not inside /media/tv: %v", err)
	}
	if len(f.refreshes) != 1 || f.refreshes[0] != "3?/media/tv/S" {
		t.Errorf("covered path refreshed once, even though another was uncovered: %q", f.refreshes)
	}
}

func TestRefreshWithPathMapping(t *testing.T) {
	f := newFake(t)
	c := newServer(t, f.srv.URL, plugin.Settings{"sectionMap": map[string]any{"/data/movies/": "/media/movies", "/data": "/media"}})
	if err := c.RefreshPaths(t.Context(), []string{"/data/movies/A (2000)", "/data/tv/B"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.refreshes, "|") != "1?/media/movies/A (2000)|3?/media/tv/B" {
		t.Errorf("%q", f.refreshes)
	}
}

func TestRefreshFailureStopsWithClass(t *testing.T) {
	f := newFake(t)
	f.failSec = "1"
	err := newServer(t, f.srv.URL, nil).RefreshPaths(t.Context(), []string{"/media/movies/x"})
	if plugin.ClassOf(err) != plugin.ClassServerError {
		t.Fatalf("%v", err)
	}
	if err := newServer(t, "http://127.0.0.1:1", nil).RefreshPaths(t.Context(), []string{"/x"}); plugin.ClassOf(err) != plugin.ClassUnreachable {
		t.Fatalf("sections unreachable: %v", err)
	}
}

func TestSettingsValidation(t *testing.T) {
	bad := []plugin.Settings{{"token": "t"}, {"url": "ftp://x", "token": "t"}, {"url": "http://x"},
		{"url": "http://x", "token": "t", "sectionMap": map[string]any{"/a": ""}}}
	for i, s := range bad {
		if _, err := New(plugin.Instance{Settings: s}); plugin.ClassOf(err) != plugin.ClassConfig {
			t.Errorf("case %d: %v", i, err)
		}
	}
}

func TestAddressStaysPinned(t *testing.T) {
	c := newServer(t, "http://plex.media.svc:32400", nil)
	if c.base.String() != "http://plex.media.svc:32400" {
		t.Errorf("%s", c.base)
	}
}

func TestPinPairingIsAnExplicitStub(t *testing.T) {
	c := newServer(t, "http://x", nil)
	if err := c.StartPinPairing(t.Context()); err != ErrPairingUnavailable {
		t.Fatal(err)
	}
	src, _ := os.ReadFile("pairing.go")
	if !strings.Contains(string(src), "TODO: Plex PIN pairing flow") {
		t.Error("stub must carry its TODO marker")
	}
}

func TestPathHelpers(t *testing.T) {
	if !hasPrefix("/a/b", "/") || hasPrefix("/ab", "/a") || !hasPrefix("/a", "/a") {
		t.Error("hasPrefix")
	}
	if trimSlash("/") != "/" || trimSlash("/a/") != "/a" {
		t.Error("trimSlash")
	}
}
