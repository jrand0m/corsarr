package qbittorrent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
)

func magnetPayload() plugin.Payload {
	return plugin.Payload{Kind: "magnet", URL: "magnet:?xt=urn:btih:" + magnetHex + "&dn=x"}
}

func TestConformance(t *testing.T) {
	reg, ok := plugin.Default.Get(ID)
	if !ok {
		t.Fatal("qbittorrent not registered")
	}
	conformance.Manifest(t, reg.Manifest, plugin.KindDownloader)
	for _, version := range []string{"v4.6.2", "v5.0.1"} {
		t.Run(version, func(t *testing.T) {
			f := newFake(t, version)
			c := newClient(t, f, nil)
			conformance.Downloader(t, c, plugin.AddRequest{Payload: magnetPayload(), Category: "movies", SavePath: "/downloads/movies"})
		})
	}
}

func TestConformanceTorrentBytes(t *testing.T) {
	f := newFake(t, "v5.0.1")
	conformance.Downloader(t, newClient(t, f, nil), plugin.AddRequest{Payload: plugin.Payload{Kind: "torrent", Content: torrentFile}, Category: "tv"})
}

func TestErrorMapping(t *testing.T) {
	conformance.ErrorMapping(t, func(url string) plugin.Tester {
		return newClient(t, &fakeQB{srv: &httptest.Server{URL: url}, user: "admin", pass: "pw"}, nil)
	},
		conformance.ErrorCase{Name: "wrong password", Want: plugin.ClassAuth,
			Handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("Fails.")) }},
		conformance.ErrorCase{Name: "login garbage", Want: plugin.ClassParse,
			Handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }},
		conformance.ErrorCase{Name: "version garbage", Want: plugin.ClassParse,
			Handler: func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "login") {
					_, _ = w.Write([]byte("Ok."))
					return
				}
				_, _ = w.Write([]byte("nonsense"))
			}},
	)
}

func TestSessionCookiesFourAndFive(t *testing.T) {
	for _, name := range []string{"SID", "QBT_SID_8080"} {
		f := newFake(t, "v5.0.1")
		f.cookieName = name
		if err := newClient(t, f, nil).Test(t.Context()); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAuthBypassNeedsNoCookie(t *testing.T) {
	f := newFake(t, "v4.6.2")
	f.bypass = true
	if err := newClient(t, f, nil).Test(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestWrongCredentials(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, plugin.Settings{"password": "nope"})
	if err := c.Test(t.Context()); plugin.ClassOf(err) != plugin.ClassAuth {
		t.Fatalf("%v", err)
	}
}

func TestRelogsInOnceOn403(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, nil)
	if err := c.Test(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.expire()
	if err := c.Test(t.Context()); err != nil {
		t.Fatalf("expired session must recover: %v", err)
	}
	if f.logins != 2 {
		t.Errorf("logins = %d, want 2", f.logins)
	}
}

func TestPersistent403IsAuthAfterOneRetry(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, nil)
	f.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "login") {
			_, _ = w.Write([]byte("Ok."))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	})
	if err := c.Test(t.Context()); plugin.ClassOf(err) != plugin.ClassAuth {
		t.Fatalf("%v", err)
	}
}

func TestSnapshotMapping(t *testing.T) {
	f := newFake(t, "v5.0.1")
	f.put("h1", map[string]any{"name": "Movie", "category": "movies", "state": "stalledDL", "progress": 0.5, "size": 1000,
		"downloaded": 500, "dlspeed": 10, "eta": 60, "num_seeds": 3, "num_leechs": 4, "save_path": "/d", "last_activity": 1700000000})
	f.put("h2", map[string]any{"name": "Done", "category": "movies", "state": "stoppedUP", "progress": 1, "eta": 8640000,
		"content_path": "/d/Done", "save_path": "/d"})
	f.put("h3", map[string]any{"name": "Bad", "category": "movies", "state": "missingFiles"})
	f.put("h4", map[string]any{"name": "Other", "category": "tv", "state": "uploading"})
	c := newClient(t, f, nil)
	snaps, err := c.List(t.Context(), []string{"movies", "movies"})
	if err != nil || len(snaps) != 3 {
		t.Fatalf("%v %d", err, len(snaps))
	}
	by := map[string]plugin.Snapshot{}
	for _, s := range snaps {
		by[s.DownloadID] = s
	}
	s1 := by["h1"]
	if s1.State != domain.StateStalled || s1.ETASeconds != 60 || s1.Seeders != 3 || s1.Peers != 4 || s1.Size != 1000 ||
		s1.Downloaded != 500 || s1.Speed != 10 || s1.SavePath != "/d" || s1.LastActivityAt == nil || s1.Progress != 0.5 || s1.Files != nil {
		t.Errorf("h1 %+v", s1)
	}
	if s2 := by["h2"]; s2.State != domain.StateCompleted || s2.ETASeconds != 0 || len(s2.Files) != 1 || s2.Files[0] != "/d/Done" {
		t.Errorf("h2 %+v", s2)
	}
	if s3 := by["h3"]; s3.State != domain.StateStalled || !strings.Contains(s3.Error, "cannot find") {
		t.Errorf("h3 %+v", s3)
	}
	all, err := c.List(t.Context(), nil)
	if err != nil || len(all) != 4 {
		t.Errorf("no category = all torrents: %v %d", err, len(all))
	}
}

func TestStateMapping(t *testing.T) {
	want := map[string]domain.DownloadState{
		"error": domain.StateStalled, "missingFiles": domain.StateStalled,
		"uploading": domain.StateCompleted, "pausedUP": domain.StateCompleted, "stoppedUP": domain.StateCompleted,
		"stalledUP": domain.StateCompleted, "queuedUP": domain.StateCompleted, "forcedUP": domain.StateCompleted,
		"downloading": domain.StateDownloading, "metaDL": domain.StateDownloading, "forcedDL": domain.StateDownloading,
		"checkingDL": domain.StateDownloading, "moving": domain.StateDownloading, "allocating": domain.StateDownloading,
		"stalledDL": domain.StateStalled, "queuedDL": domain.StateQueued, "pausedDL": domain.StateQueued,
		"stoppedDL": domain.StateQueued, "somethingNew": domain.StateQueued,
	}
	for in, w := range want {
		if got := mapState(in); got != w {
			t.Errorf("%s → %s, want %s", in, got, w)
		}
	}
}

func TestListBadJSON(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, nil)
	inner := f.srv.Config.Handler
	f.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "torrents/info") {
			_, _ = w.Write([]byte("{"))
			return
		}
		inner.ServeHTTP(w, r)
	})
	if _, err := c.List(t.Context(), nil); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("list: %v", err)
	}
	if _, err := c.Add(t.Context(), plugin.AddRequest{Payload: magnetPayload()}); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("exists check: %v", err)
	}
}

func TestAddByMagnetReturnsHash(t *testing.T) {
	f := newFake(t, "v5.0.1")
	c := newClient(t, f, nil)
	id, err := c.Add(t.Context(), plugin.AddRequest{Payload: magnetPayload(), Category: "movies", SavePath: "/m", Paused: true})
	if err != nil || id != magnetHex {
		t.Fatalf("%q %v", id, err)
	}
	form := f.forms[0]
	if form["category"] != "movies" || form["savepath"] != "/m" || form["autoTMM"] != "false" || form["stopped"] != "true" || form["paused"] != "" {
		t.Errorf("5.x form %v", form)
	}
}

func TestAddPausedFourX(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, nil)
	if _, err := c.Add(t.Context(), plugin.AddRequest{Payload: magnetPayload(), Paused: true}); err != nil {
		t.Fatal(err)
	}
	if form := f.forms[0]; form["paused"] != "true" || form["stopped"] != "" || form["category"] != "" || form["savepath"] != "" {
		t.Errorf("4.x form %v", form)
	}
}

func TestAddTorrentBytesHash(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, nil)
	id, err := c.Add(t.Context(), plugin.AddRequest{Payload: plugin.Payload{Kind: "torrent", Content: torrentFile}})
	if err != nil || id != fixtureHash() {
		t.Fatalf("%q %v want %s", id, err, fixtureHash())
	}
}

func TestAddByURLFetchesTorrent(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, nil)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(torrentFile)
	}))
	defer src.Close()
	id, err := c.Add(t.Context(), plugin.AddRequest{Payload: plugin.Payload{Kind: "torrent", URL: src.URL + "/x.torrent"}})
	if err != nil || id != fixtureHash() {
		t.Fatalf("%q %v", id, err)
	}
	if _, err := c.Add(t.Context(), plugin.AddRequest{Payload: plugin.Payload{URL: src.URL + "/bad"}}); plugin.ClassOf(err) != plugin.ClassServerError {
		t.Errorf("download failure: %v", err)
	}
	if _, err := c.Add(t.Context(), plugin.AddRequest{Payload: plugin.Payload{URL: "ftp://x/y"}}); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("ftp: %v", err)
	}
	if _, err := c.Add(t.Context(), plugin.AddRequest{Payload: plugin.Payload{URL: "http://[bad"}}); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("bad url: %v", err)
	}
}

func TestAddRejectsBadPayloads(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, nil)
	bad := []plugin.Payload{
		{}, {Content: []byte("not a torrent")}, {URL: "magnet:?dn=nohash"}, {URL: "magnet:?xt=urn:btih:zz"},
		{Content: []byte("d4:infod4:name1:x")},
	}
	for i, p := range bad {
		if _, err := c.Add(t.Context(), plugin.AddRequest{Payload: p}); plugin.ClassOf(err) != plugin.ClassParse {
			t.Errorf("case %d: %v", i, err)
		}
	}
	if len(f.forms) != 0 {
		t.Error("nothing may reach the client")
	}
}

func TestAddRefusedByClient(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, nil)
	inner := f.srv.Config.Handler
	f.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "torrents/add") {
			_, _ = w.Write([]byte("Fails."))
			return
		}
		inner.ServeHTTP(w, r)
	})
	if _, err := c.Add(t.Context(), plugin.AddRequest{Payload: magnetPayload()}); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("%v", err)
	}
}

func TestAddIdempotentWhenAlreadyPresent(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, nil)
	for range 2 {
		if id, err := c.Add(t.Context(), plugin.AddRequest{Payload: magnetPayload()}); err != nil || id != magnetHex {
			t.Fatalf("%q %v", id, err)
		}
	}
	if len(f.forms) != 1 {
		t.Errorf("second add must not post: %d", len(f.forms))
	}
}

func TestAddRaceReturnsHashWhenClientSaysFails(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, nil)
	inner := f.srv.Config.Handler
	hits := 0
	f.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "torrents/info") && r.URL.Query().Get("hashes") != "" {
			hits++
			if hits == 1 {
				_, _ = w.Write([]byte("[]"))
				return
			}
			_, _ = w.Write([]byte(`[{"hash":"` + magnetHex + `"}]`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "torrents/add") {
			_, _ = w.Write([]byte("Fails."))
			return
		}
		inner.ServeHTTP(w, r)
	})
	if id, err := c.Add(t.Context(), plugin.AddRequest{Payload: magnetPayload()}); err != nil || id != magnetHex {
		t.Fatalf("%q %v", id, err)
	}
}

func TestPauseResumeByVersion(t *testing.T) {
	cases := map[string][2]string{"v4.6.2": {"torrents/pause", "torrents/resume"}, "v5.0.1": {"torrents/stop", "torrents/start"}}
	for version, paths := range cases {
		f := newFake(t, version)
		c := newClient(t, f, nil)
		if err := c.Pause(t.Context(), "ABC"); err != nil {
			t.Fatalf("%s pause: %v", version, err)
		}
		if err := c.Resume(t.Context(), "ABC"); err != nil {
			t.Fatalf("%s resume: %v", version, err)
		}
		if !f.called(paths[0]) || !f.called(paths[1]) {
			t.Errorf("%s calls %v", version, f.calls)
		}
	}
}

func TestPauseFailsWhenVersionUnknown(t *testing.T) {
	c := newClient(t, newFake(t, "v4.6.2"), plugin.Settings{"password": "bad"})
	if err := c.Pause(t.Context(), "h"); plugin.ClassOf(err) != plugin.ClassAuth {
		t.Fatalf("%v", err)
	}
}

func TestSetPriority(t *testing.T) {
	f := newFake(t, "v5.0.1")
	c := newClient(t, f, nil)
	want := map[plugin.Priority]string{plugin.PrioTop: "topPrio", plugin.PrioBottom: "bottomPrio", plugin.PrioUp: "increasePrio", plugin.PrioDown: "decreasePrio"}
	for p, path := range want {
		if err := c.SetPriority(t.Context(), "h", p); err != nil || !f.called("torrents/"+path) {
			t.Errorf("%s: %v", p, err)
		}
	}
	if err := c.SetPriority(t.Context(), "h", "sideways"); plugin.ClassOf(err) != plugin.ClassConfig {
		t.Errorf("unknown priority: %v", err)
	}
	f.noQueueing = true
	if err := c.SetPriority(t.Context(), "h", plugin.PrioTop); plugin.ClassOf(err) != plugin.ClassConfig {
		t.Errorf("queueing disabled: %v", err)
	}
}

func TestOtherClientErrorsSurface(t *testing.T) {
	f := newFake(t, "v5.0.1")
	c := newClient(t, f, nil)
	f.noQueueing = true
	// 409 on a non-priority call is a plain server error, not the queueing hint.
	if _, err := c.postForm(t.Context(), "torrents/topPrio", nil); plugin.ClassOf(err) != plugin.ClassServerError {
		t.Errorf("%v", err)
	}
	if _, err := c.postForm(t.Context(), "nonexistent", nil); err == nil {
		t.Error("404 must be an error")
	}
}

func TestRemoveFlagsFiles(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, nil)
	f.put("abc", map[string]any{"category": "tv", "state": "uploading"})
	if err := c.Remove(t.Context(), "ABC", true); err != nil || !f.called("deleteFiles=true") {
		t.Fatalf("%v %v", err, f.calls)
	}
	if err := c.Remove(t.Context(), "ABC", false); err != nil || !f.called("deleteFiles=false") {
		t.Fatalf("repeat remove: %v", err)
	}
}

func TestEnsureCategory(t *testing.T) {
	f := newFake(t, "v4.6.2")
	c := newClient(t, f, nil)
	ctx := t.Context()
	if err := c.EnsureCategory(ctx, "movies", "/m"); err != nil || f.categories["movies"] != "/m" || !f.called("torrents/createCategory") {
		t.Fatalf("create: %v", err)
	}
	if err := c.EnsureCategory(ctx, "movies", "/m"); err != nil || f.called("torrents/editCategory") {
		t.Fatal("unchanged category must not be edited")
	}
	if err := c.EnsureCategory(ctx, "movies", "/other"); err != nil || f.categories["movies"] != "/other" || !f.called("torrents/editCategory") {
		t.Fatal("edit")
	}
	if err := c.EnsureCategory(ctx, "movies", ""); err != nil || f.categories["movies"] != "/other" {
		t.Fatal("empty save path keeps the current one")
	}
	inner := f.srv.Config.Handler
	f.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "categories") {
			_, _ = w.Write([]byte("["))
			return
		}
		inner.ServeHTTP(w, r)
	})
	if err := c.EnsureCategory(ctx, "x", ""); plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("bad json: %v", err)
	}
}

func TestSettingsAndCategories(t *testing.T) {
	for _, s := range []plugin.Settings{{"url": ""}, {"url": "ftp://h"}, {"url": "http://h", "categories": map[string]any{"x": "y"}},
		{"url": "http://h", "categories": map[string]any{"tv": ""}}} {
		if _, err := New(plugin.Instance{Settings: s}); plugin.ClassOf(err) != plugin.ClassConfig {
			t.Errorf("%v: %v", s, err)
		}
	}
	p, err := New(plugin.Instance{Settings: plugin.Settings{"url": "http://h:8080/sub/", "categories": map[string]any{"movie": "films"}}})
	if err != nil {
		t.Fatal(err)
	}
	c := p.(*Client)
	if c.CategoryFor(domain.KindMovie) != "films" || c.CategoryFor(domain.KindTV) != "tv" || c.CategoryFor(domain.KindBook) != "books" ||
		c.CategoryFor(domain.KindMusic) != "music" {
		t.Errorf("%v", c.categories)
	}
	if got := c.endpoint("app/version", nil); got != "http://h:8080/sub/api/v2/app/version" {
		t.Errorf("sub-path endpoint %s", got)
	}
	if len(c.Protocols()) != 1 || c.Protocols()[0] != domain.ProtoTorrent {
		t.Error("protocols")
	}
}

func TestHashHelpers(t *testing.T) {
	if h, err := magnetHash("magnet:?xt=urn:btih:" + strings.ToUpper(magnetHex)); err != nil || h != magnetHex {
		t.Errorf("upper hex: %q %v", h, err)
	}
	// 20 zero bytes in base32 is 32 'A' characters.
	if h, err := magnetHash("magnet:?xt=urn:btih:" + strings.Repeat("a", 32)); err != nil || h != strings.Repeat("0", 40) {
		t.Errorf("base32: %q %v", h, err)
	}
	for _, bad := range []string{"http://x", "magnet:?xt=urn:sha1:abc", "magnet:?xt=urn:btih:" + strings.Repeat("g", 40), "magnet:?xt=urn:btih:" + strings.Repeat("1", 32), "magnet:?xt=urn:btih:abc"} {
		if _, err := magnetHash(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	for _, bad := range []string{"", "x", "d", "dx", "d3:foo", "d3:fooi5", "d3:fool", "d3:fooli1e", "d99:x", "d3:foo3:ba", "d3:fooxe", "d4:spami1ee", "d-1:x", "dl" + strings.Repeat("l", 40) + "e"} {
		if _, err := torrentHash([]byte(bad)); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	nested := []byte("d4:infod1:ali1ei2ee1:bl1:xee4:name1:ye")
	if _, err := torrentHash(nested); err != nil {
		t.Errorf("nested containers: %v", err)
	}
}
