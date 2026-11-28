package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSessionHandshakeRetriesOnceAndKeepsTheID(t *testing.T) {
	f := newFake(t, "4.0.6 (38c164933e)", 17)
	c := f.client(t, Config{})
	for range 3 {
		if _, err := c.List(tctx(t), nil); err != nil {
			t.Fatal(err)
		}
	}
	if f.handshakeCount() != 1 {
		t.Errorf("handshakes = %d, want 1 (the id is kept)", f.handshakeCount())
	}
	f.mu.Lock()
	f.sessionID = "sess-2" // the server restarted
	f.mu.Unlock()
	if _, err := c.List(tctx(t), nil); err != nil {
		t.Fatalf("after session change: %v", err)
	}
	if f.handshakeCount() != 2 || c.currentSessionID() != "sess-2" {
		t.Errorf("handshakes = %d, id %q", f.handshakeCount(), c.currentSessionID())
	}
}

func TestSessionHandshakeGivesUpAfterOneRetry(t *testing.T) {
	f := newFake(t, "4.0.6 (38c164933e)", 17)
	f.alwaysStale = true
	_, err := f.client(t, Config{}).Test(tctx(t))
	if ClassOf(err) != ClassServerError || !strings.Contains(err.Error(), "session id") {
		t.Fatalf("err = %v", err)
	}
	if f.handshakeCount() != 2 {
		t.Errorf("handshakes = %d, want 2", f.handshakeCount())
	}
}

func TestBasicAuth(t *testing.T) {
	f := newFake(t, "3.00 (bb6b5a062e)", 16)
	f.user, f.pass = "admin", "secret"
	if _, err := f.client(t, Config{}).Test(tctx(t)); err != nil {
		t.Fatalf("right password: %v", err)
	}
	_, err := f.client(t, Config{Username: "admin", Password: "wrong"}).Test(tctx(t))
	if ClassOf(err) != ClassAuth || !strings.Contains(err.Error(), "user name or password") {
		t.Fatalf("wrong password: %v", err)
	}
}

func handlerClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(httpDoer{srv.Client()}, Config{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }
}

func body(s string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(s)) }
}

// TestErrorMapping mirrors the host's error mapping checks: every failure of
// Test comes back with its class.
func TestErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		h    http.HandlerFunc
		want Class
	}{
		{"401", status(401), ClassAuth},
		{"403 whitelist", status(403), ClassAuth},
		{"429", status(429), ClassRateLimited},
		{"500", status(500), ClassServerError},
		{"503", status(503), ClassServerError},
		{"404", status(404), ClassServerError},
		{"hang", func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(300 * time.Millisecond):
			}
		}, ClassTimeout},
		{"garbage", body("<html>"), ClassParse},
		{"no result", body(`{"arguments":{}}`), ClassParse},
		{"bad arguments", body(`{"result":"success","arguments":{"rpc-version":"x"}}`), ClassParse},
		{"refused", body(`{"result":"no such directory"}`), ClassServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			if _, err := handlerClient(t, tc.h).Test(ctx); ClassOf(err) != tc.want {
				t.Errorf("class %q (%v), want %q", ClassOf(err), err, tc.want)
			}
		})
	}
	t.Run("unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		c, _ := New(httpDoer{http.DefaultClient}, Config{URL: url})
		if _, err := c.Test(tctx(t)); ClassOf(err) != ClassUnreachable {
			t.Errorf("class %q, want unreachable", ClassOf(err))
		}
	})
}

func TestConfiguredTimeoutBoundsARequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(300 * time.Millisecond):
		}
	}))
	defer srv.Close()
	c, _ := New(httpDoer{srv.Client()}, Config{URL: srv.URL, Timeout: 50 * time.Millisecond})
	if _, err := c.Test(context.Background()); ClassOf(err) != ClassTimeout {
		t.Fatalf("err = %v", err)
	}
}

func TestResultStringsAreReported(t *testing.T) {
	f := newFake(t, "4.0.6 (38c164933e)", 17)
	c := f.client(t, Config{})
	_, err := c.Add(tctx(t), AddRequest{Torrent: []byte("not bencode")})
	if ClassOf(err) != ClassParse || !strings.Contains(err.Error(), "invalid or corrupt torrent file") {
		t.Fatalf("corrupt torrent: %v", err)
	}
	f.results["free-space"] = "No such file or directory (/downloads)"
	_, err = c.Test(tctx(t))
	if ClassOf(err) != ClassServerError || !strings.Contains(err.Error(), "No such file or directory") {
		t.Fatalf("free-space: %v", err)
	}
	st, err := c.Stats(tctx(t))
	if err != nil || st.FreeSpace != -1 {
		t.Fatalf("stats with unreadable free space: %+v %v", st, err)
	}
}

func TestLargeListIsOneBatchedRequestWithOnlyListFields(t *testing.T) {
	f := newFake(t, "4.0.6 (38c164933e)", 17)
	for i := range 5000 {
		cat := []string{"movies", "tv"}[i%2]
		f.add(&fakeTorrent{hash: fmt.Sprintf("%040x", i), name: fmt.Sprint("t", i), dir: "/downloads", labels: []string{cat},
			status: statusSeed, percent: 1, size: 10})
	}
	c := f.client(t, Config{})
	if _, err := c.Test(tctx(t)); err != nil {
		t.Fatal(err)
	}
	before := len(f.methods())
	list, err := c.List(tctx(t), []string{"tv", "movies"})
	if err != nil || len(list) != 5000 {
		t.Fatalf("list: %d %v", len(list), err)
	}
	calls := f.methods()[before:]
	if !slices.Equal(calls, []string{"torrent-get"}) {
		t.Errorf("calls = %v, want one torrent-get", calls)
	}
	fields := f.args("torrent-get")["fields"].([]any)
	for _, heavy := range []string{"files", "trackerStats", "peers", "pieces"} {
		if slices.Contains(fields, any(heavy)) {
			t.Errorf("list asks for %s", heavy)
		}
	}
	if _, ok := f.args("torrent-get")["ids"]; ok {
		t.Error("list filters by ids")
	}
	tv, _ := c.List(tctx(t), []string{"tv"})
	if len(tv) != 2500 || tv[0].Category != "tv" {
		t.Errorf("tv: %d", len(tv))
	}
}

func TestEmptyIDNeverAddressesEveryTorrent(t *testing.T) {
	f := newFake(t, "4.0.6 (38c164933e)", 17)
	c := f.client(t, Config{})
	for name, op := range map[string]func() error{
		"remove":   func() error { return c.Remove(tctx(t), " ", true) },
		"pause":    func() error { return c.Pause(tctx(t), "") },
		"priority": func() error { return c.SetPriority(tctx(t), "", PrioTop) },
		"files":    func() error { _, err := c.Files(tctx(t), ""); return err },
	} {
		if err := op(); ClassOf(err) != ClassConfig {
			t.Errorf("%s: %v", name, err)
		}
	}
	if calls := f.methods(); len(calls) != 0 {
		t.Errorf("requests sent: %v", calls)
	}
}

func TestAddArguments(t *testing.T) {
	for _, v := range fakeVersions {
		t.Run(v.name, func(t *testing.T) {
			f := newFake(t, v.version, v.rpc)
			c := f.client(t, Config{AddPaused: true, CategoryDirs: map[string]string{"tv": "/data/tv"},
				PathMap: map[string]string{"/data": "/mnt/data"}})
			id, err := c.Add(tctx(t), AddRequest{URL: "magnet:?xt=urn:btih:" + strings.ToUpper(magnetHex), Category: "tv"})
			must(t, "Add", err)
			args := f.args("torrent-add")
			if args["download-dir"] != "/data/tv" || args["paused"] != true {
				t.Errorf("args = %v", args)
			}
			_, hasLabels := args["labels"]
			if hasLabels != (v.rpc >= 17) {
				t.Errorf("labels sent on add: %v (rpc %d)", hasLabels, v.rpc)
			}
			list, _ := c.List(tctx(t), []string{"tv"})
			if len(list) != 1 || list[0].ID != id || list[0].State != StatePaused || list[0].SavePath != "/mnt/data/tv" ||
				!slices.Equal(list[0].Labels, []string{"tv"}) {
				t.Errorf("list = %+v", list)
			}
			_, err = c.Add(tctx(t), AddRequest{Torrent: torrentBytes, SavePath: "/mnt/data/other"})
			must(t, "Add with save path", err)
			if got := f.args("torrent-add")["download-dir"]; got != "/data/other" {
				t.Errorf("save path sent as %v", got)
			}
		})
	}
}

func TestAddRejectsBadPayloads(t *testing.T) {
	c := newFake(t, "4.0.6 (38c164933e)", 17).client(t, Config{})
	for _, req := range []AddRequest{{}, {URL: "ftp://x/y.torrent"}, {URL: "magnet:?xt=x", Category: "a,b"}} {
		if _, err := c.Add(tctx(t), req); ClassOf(err) == "" {
			t.Errorf("%+v: %v", req, err)
		}
	}
}

func TestAddWithoutHashInAnswer(t *testing.T) {
	c := handlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":"success","arguments":{"version":"4.0.6","rpc-version":17}}`))
	})
	if _, err := c.Add(tctx(t), AddRequest{URL: "magnet:?xt=urn:btih:" + magnetHex}); ClassOf(err) != ClassParse {
		t.Fatalf("err = %v", err)
	}
}

func TestCategoryNeedsDirectoryWithoutLabels(t *testing.T) {
	f := newFake(t, "2.94 (d8e60ee44f)", 15)
	c := f.client(t, Config{})
	if err := c.EnsureCategory(tctx(t), "tv", ""); ClassOf(err) != ClassConfig {
		t.Errorf("ensure: %v", err)
	}
	if _, err := c.Add(tctx(t), AddRequest{URL: "magnet:?xt=urn:btih:" + magnetHex, Category: "tv"}); ClassOf(err) != ClassConfig {
		t.Errorf("add: %v", err)
	}
	id, err := c.Add(tctx(t), AddRequest{URL: "magnet:?xt=urn:btih:" + magnetHex})
	must(t, "add without category", err)
	if err := c.SetCategory(tctx(t), id, "tv"); !errors.Is(err, ErrUnsupported) || ClassOf(err) != "" {
		t.Errorf("set category: %v", err)
	}
	if err := c.EnsureCategory(tctx(t), "", "/x"); ClassOf(err) != ClassConfig {
		t.Errorf("empty category: %v", err)
	}
}

func TestSetCategoryKeepsOtherLabels(t *testing.T) {
	f := newFake(t, "4.0.6 (38c164933e)", 17)
	f.add(&fakeTorrent{hash: magnetHex, name: "x", dir: "/downloads", labels: []string{"keep", "movies"}})
	c := f.client(t, Config{CategoryDirs: map[string]string{"movies": "/downloads/movies"}})
	must(t, "SetCategory", c.SetCategory(tctx(t), strings.ToUpper(magnetHex), "tv"))
	if got := f.snapshot(0).labels; !slices.Equal(got, []string{"keep", "tv"}) {
		t.Errorf("labels = %v", got)
	}
	must(t, "clear", c.SetCategory(tctx(t), magnetHex, ""))
	if got := f.snapshot(0).labels; !slices.Equal(got, []string{"keep"}) {
		t.Errorf("labels = %v", got)
	}
	if err := c.SetCategory(tctx(t), "ffff", "tv"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown torrent: %v", err)
	}
}

func TestPriorityLevels(t *testing.T) {
	f := newFake(t, "4.0.6 (38c164933e)", 17)
	f.add(&fakeTorrent{hash: magnetHex, name: "x", status: statusDownloadWait})
	c := f.client(t, Config{})
	for level, want := range map[Priority]int{PrioLow: -1, PrioHigh: 1, PrioNormal: 0} {
		must(t, string(level), c.SetPriorityLevel(tctx(t), magnetHex, level))
		if f.snapshot(0).bp != want {
			t.Errorf("%s: bandwidthPriority %d", level, f.snapshot(0).bp)
		}
	}
	must(t, "force", c.SetPriorityLevel(tctx(t), magnetHex, PrioForce))
	if !slices.Contains(f.methods(), "torrent-start-now") {
		t.Error("force did not start now")
	}
	if err := c.SetPriorityLevel(tctx(t), magnetHex, PrioTop); ClassOf(err) != ClassConfig {
		t.Errorf("move as level: %v", err)
	}
	if err := c.SetPriority(tctx(t), magnetHex, PrioHigh); ClassOf(err) != ClassConfig {
		t.Errorf("level as move: %v", err)
	}
}

func TestQueueMovesReorder(t *testing.T) {
	f := newFake(t, "3.00 (bb6b5a062e)", 16)
	for _, h := range []string{"a", "b", "c"} {
		f.add(&fakeTorrent{hash: strings.Repeat(h, 40), name: h})
	}
	c := f.client(t, Config{})
	must(t, "bottom", c.SetPriority(tctx(t), strings.Repeat("a", 40), PrioBottom))
	must(t, "up", c.SetPriority(tctx(t), strings.Repeat("c", 40), PrioTop))
	list, _ := c.List(tctx(t), nil)
	var order []string
	for _, tr := range list {
		order = append(order, tr.Name+fmt.Sprint(tr.QueuePosition))
	}
	if !slices.Equal(order, []string{"c0", "b1", "a2"}) {
		t.Errorf("order = %v", order)
	}
}
