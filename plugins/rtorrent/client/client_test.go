package client

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

func must(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

var profiles = []profile{v098, v010, legacy}

// TestLifecycle mirrors the downloader conformance checks: test, ensure
// category twice, idempotent add, list, control, priority moves, remove.
func TestLifecycle(t *testing.T) {
	adds := map[string]AddRequest{
		"magnet":  {Magnet: magnet, Label: "tv", Directory: "/data/tv"},
		"torrent": {Torrent: torrentFile, Label: "tv", Directory: "/data/tv"},
	}
	for _, p := range profiles {
		for kind, add := range adds {
			t.Run(p.version+"/"+kind, func(t *testing.T) {
				f := newFake(t, p)
				c := f.client(t, func(cfg *Config) {
					cfg.PathMap = PathMap{{Remote: "/data", Local: "/mnt/media"}}
					add.Directory = "/mnt/media/tv"
				})
				lifecycle(t, f, c, add)
			})
		}
	}
}

func lifecycle(t *testing.T, f *fakeRT, c *Client, add AddRequest) {
	t.Helper()
	v, err := c.Version(ctx(t))
	must(t, "Version", err)
	if v.Client != f.prof.version {
		t.Errorf("version %q", v.Client)
	}
	for range 2 {
		must(t, "EnsureLabel", c.EnsureLabel(ctx(t), add.Label))
	}
	id, err := c.Add(ctx(t), add)
	must(t, "Add", err)
	again, err := c.Add(ctx(t), add)
	if err != nil || again != id {
		t.Errorf("Add is not idempotent: %q then %q (%v)", id, again, err)
	}
	if len(f.loads) != 1 {
		t.Errorf("%d loads, want 1", len(f.loads))
	}
	if ft := f.get(id); ft == nil || ft.dir != "/data/tv" || ft.custom["custom1"] != "tv" {
		t.Fatalf("loaded torrent %+v", ft)
	}
	if !listed(t, c, add.Label, id) {
		t.Fatalf("download %s missing from List", id)
	}
	if listed(t, c, "movies", id) {
		t.Error("download listed under another label")
	}
	control(t, c, id)
	must(t, "Remove", c.Remove(ctx(t), id, f.prof.erasedata))
	if listed(t, c, add.Label, id) {
		t.Error("still listed after Remove")
	}
	must(t, "Remove again", c.Remove(ctx(t), id, false))
}

func control(t *testing.T, c *Client, id string) {
	t.Helper()
	must(t, "Pause", c.Pause(ctx(t), id))
	must(t, "Resume", c.Resume(ctx(t), id))
	for _, m := range []string{"top", "up", "down", "bottom"} {
		must(t, "Move "+m, c.Move(ctx(t), id, m))
	}
}

func listed(t *testing.T, c *Client, label, id string) bool {
	t.Helper()
	ts, err := c.List(ctx(t), []string{label})
	must(t, "List", err)
	for _, x := range ts {
		if x.Hash == id {
			return true
		}
	}
	return false
}

func TestAddPausedAndState(t *testing.T) {
	f := newFake(t, v098)
	c := f.client(t)
	id, err := c.Add(ctx(t), AddRequest{Magnet: magnet, Paused: true})
	must(t, "Add", err)
	if f.loads[0][1] != magnet || !f.called("load.normal") {
		t.Errorf("paused magnet not loaded with load.normal: %v", f.calls)
	}
	ts, _ := c.List(ctx(t), nil)
	if len(ts) != 1 || ts[0].State != StatePaused || ts[0].Name != "Some.Show" {
		t.Fatalf("list %+v", ts)
	}
	must(t, "Resume", c.Resume(ctx(t), id))
	if ft := f.get(id); !ft.started || !ft.active {
		t.Errorf("resume did not start a stopped download: %+v", ft)
	}
	must(t, "Pause", c.Pause(ctx(t), id))
	if ft := f.get(id); !ft.started || ft.active {
		t.Errorf("pause should keep the download started: %+v", ft)
	}
	must(t, "Pause again", c.Pause(ctx(t), id))
	must(t, "Resume paused", c.Resume(ctx(t), id))
	must(t, "Resume running", c.Resume(ctx(t), id))
	must(t, "Stop", c.Stop(ctx(t), id))
	if f.get(id).started {
		t.Error("stop left the download started")
	}
	must(t, "Start", c.Start(ctx(t), id))
	_, err = c.Add(ctx(t), AddRequest{Torrent: torrentFile, Paused: true})
	must(t, "Add raw paused", err)
	if !f.called("load.raw") {
		t.Error("paused torrent not loaded with load.raw")
	}
}

func TestAddFromURL(t *testing.T) {
	f := newFake(t, v010)
	files := newFileServer(t, torrentFile)
	c := f.client(t)
	id, err := c.Add(ctx(t), AddRequest{URL: files.URL + "/x.torrent"})
	must(t, "Add", err)
	if id != fixtureHash() || !f.called("load.raw_start") {
		t.Errorf("id %s calls %v", id, f.calls)
	}
	if _, err := c.Add(ctx(t), AddRequest{URL: magnet}); err != nil {
		t.Errorf("magnet in URL: %v", err)
	}
	bad := []AddRequest{{}, {URL: "ftp://x/y"}, {URL: files.URL + "/missing"}, {Magnet: "magnet:?xt=urn:sha1:x"},
		{Torrent: []byte("not bencode")}, {URL: "http://127.0.0.1:1/x.torrent"}}
	for _, req := range bad {
		if _, err := c.Add(ctx(t), req); err == nil {
			t.Errorf("Add(%+v) succeeded", req)
		}
	}
}

func TestAddNotLoaded(t *testing.T) {
	f := newFake(t, v098)
	f.raw["load.start"] = mustEncode(t, int64(0)) // accepted but nothing created
	_, err := f.client(t).Add(ctx(t), AddRequest{Magnet: magnet})
	if KindOf(err) != KindServer {
		t.Fatalf("want server error, got %v", err)
	}
}

func TestQuotedLoadCommands(t *testing.T) {
	f := newFake(t, v098)
	c := f.client(t, func(cfg *Config) { cfg.LabelField = "custom2" })
	id, err := c.Add(ctx(t), AddRequest{Magnet: magnet, Directory: `/data/a "b", c\d`, Label: "TV Shows/HD"})
	must(t, "Add", err)
	ft := f.get(id)
	if ft.dir != `/data/a "b", c\d` || ft.custom["custom2"] != "TV%20Shows%2FHD" {
		t.Errorf("dir %q label %q", ft.dir, ft.custom["custom2"])
	}
	ts, _ := c.List(ctx(t), []string{"TV Shows/HD"})
	if len(ts) != 1 || ts[0].Label != "TV Shows/HD" {
		t.Errorf("label not decoded: %+v", ts)
	}
	must(t, "SetLabel", c.SetLabel(ctx(t), id, "movies"))
	if ft.custom["custom2"] != "movies" {
		t.Errorf("label %q", ft.custom["custom2"])
	}
}

func TestRemoveWithData(t *testing.T) {
	f := newFake(t, v098)
	c := f.client(t)
	id, _ := c.Add(ctx(t), AddRequest{Magnet: magnet})
	ft := f.get(id)
	ok, err := c.SupportsEraseData(ctx(t))
	if err != nil || !ok {
		t.Fatalf("erase data support %v %v", ok, err)
	}
	must(t, "Remove", c.Remove(ctx(t), id, true))
	if !ft.erasedData {
		t.Error("data not flagged for erasing")
	}
}

func TestRemoveWithDataUnsupported(t *testing.T) {
	f := newFake(t, legacy)
	c := f.client(t)
	id, _ := c.Add(ctx(t), AddRequest{Magnet: magnet})
	err := c.Remove(ctx(t), id, true)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("want unsupported, got %v", err)
	}
	if f.get(id) == nil {
		t.Fatal("download removed although its data could not be")
	}
	if ok, _ := c.SupportsEraseData(ctx(t)); ok {
		t.Error("legacy fake reports erase support")
	}
	must(t, "Remove without data", c.Remove(ctx(t), id, false))
}

func TestEraseProbeTransportError(t *testing.T) {
	f := newFake(t, v098)
	c := f.client(t)
	f.status = 502
	if _, err := c.SupportsEraseData(ctx(t)); KindOf(err) != KindServer {
		t.Fatalf("want server error, got %v", err)
	}
	f.status = 0
	f.raw["method.get"] = EncodeFault(-503, "Key not found.")
	if ok, err := c.SupportsEraseData(ctx(t)); ok || err != nil {
		t.Fatalf("fault should mean no handler: %v %v", ok, err)
	}
}

func TestStates(t *testing.T) {
	cases := []struct {
		name string
		t    fakeTorrent
		want State
	}{
		{"checking", fakeTorrent{checking: true, started: true}, StateChecking},
		{"stopped complete", fakeTorrent{complete: true, msg: "Tracker: timeout"}, StateCompleted},
		{"stopped with error", fakeTorrent{msg: "Storage error: disk full"}, StateFailed},
		{"stopped", fakeTorrent{}, StatePaused},
		{"queued", fakeTorrent{started: true}, StateQueued},
		{"paused", fakeTorrent{started: true, open: true}, StatePaused},
		{"seeding", fakeTorrent{started: true, open: true, active: true, complete: true}, StateSeeding},
		{"downloading", fakeTorrent{started: true, open: true, active: true, down: 10}, StateDownloading},
		{"stalled", fakeTorrent{started: true, open: true, active: true, msg: "Tracker: refused"}, StateStalled},
	}
	f := newFake(t, v098)
	for i, tc := range cases {
		ft := tc.t
		ft.hash = strings.Repeat(string(rune('A'+i)), 40)
		ft.name = tc.name
		f.add(&ft)
	}
	ts, err := f.client(t).List(ctx(t), nil)
	must(t, "List", err)
	got := map[string]State{}
	for _, x := range ts {
		got[x.Name] = x.State
	}
	for _, tc := range cases {
		if got[tc.name] != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got[tc.name], tc.want)
		}
	}
}

func TestListFields(t *testing.T) {
	f := newFake(t, v010)
	f.add(&fakeTorrent{hash: "AB" + strings.Repeat("0", 38), name: "Show", dir: "/data/tv/Show", multi: true,
		started: true, open: true, active: true, size: 1000, done: 250, down: 25, up: 5, ratio: 1500, seeders: 3,
		prio: 3, custom: map[string]string{"custom1": "tv"}})
	f.add(&fakeTorrent{hash: "CD" + strings.Repeat("0", 38), name: "film.mkv", dir: "/data/movies",
		complete: true, size: 10, done: 10})
	c := f.client(t, func(cfg *Config) { cfg.PathMap = PathMap{{Remote: "/data", Local: "/mnt/media"}} })
	ts, err := c.List(ctx(t), nil)
	must(t, "List", err)
	a, b := ts[0], ts[1]
	if a.Label != "tv" || a.Progress != 0.25 || a.ETASeconds != 30 || a.Ratio != 1.5 || a.Seeders != 3 ||
		a.Peers != 6 || a.Priority != PriorityHigh || a.UpRate != 5 {
		t.Errorf("fields %+v", a)
	}
	if a.SavePath != "/mnt/media/tv" || a.ContentPath != "/mnt/media/tv/Show" {
		t.Errorf("multi-file paths %q %q", a.SavePath, a.ContentPath)
	}
	if b.SavePath != "/mnt/media/movies" || b.ContentPath != "/mnt/media/movies/film.mkv" || b.State != StateCompleted {
		t.Errorf("single-file %+v", b)
	}
}

func TestLargeMulticall(t *testing.T) {
	f := newFake(t, v098)
	for i := range 3000 {
		h := strings.ToUpper(hex40(i))
		f.add(&fakeTorrent{hash: h, name: "release " + h, dir: "/downloads", size: 1 << 30,
			custom: map[string]string{"custom1": []string{"tv", "movies"}[i%2]}})
	}
	c := f.client(t)
	ts, err := c.List(ctx(t), []string{"tv"})
	must(t, "List", err)
	if len(ts) != 1500 {
		t.Fatalf("%d torrents, want 1500", len(ts))
	}
	small := f.client(t, func(cfg *Config) { cfg.MaxResponse = 1 << 20 })
	if _, err := small.List(ctx(t), nil); KindOf(err) != KindParse {
		t.Fatalf("oversized answer: %v", err)
	}
}

func hex40(i int) string {
	const digits = "0123456789abcdef"
	b := []byte(strings.Repeat("0", 40))
	for p := 39; i > 0; p-- {
		b[p] = digits[i%16]
		i /= 16
	}
	return string(b)
}

func TestPriority(t *testing.T) {
	f := newFake(t, v098)
	c := f.client(t)
	id, _ := c.Add(ctx(t), AddRequest{Magnet: magnet})
	steps := []struct {
		move string
		want int64
	}{{"top", 3}, {"up", 3}, {"down", 2}, {"down", 1}, {"down", 1}, {"up", 2}, {"bottom", 1}}
	for _, s := range steps {
		must(t, s.move, c.Move(ctx(t), id, s.move))
		if got := f.get(id).prio; got != s.want {
			t.Errorf("after %s: %d, want %d", s.move, got, s.want)
		}
	}
	if err := c.Move(ctx(t), id, "sideways"); KindOf(err) != KindConfig {
		t.Errorf("unknown move: %v", err)
	}
	if err := c.SetPriority(ctx(t), id, 4); KindOf(err) != KindConfig {
		t.Errorf("priority 4: %v", err)
	}
	for name, want := range map[string]Priority{"off": 0, "low": 1, "normal": 2, "high": 3, "force": 3} {
		p, err := Level(name)
		if err != nil || p != want {
			t.Errorf("Level(%s) = %d %v", name, p, err)
		}
	}
	if _, err := Level("urgent"); err == nil {
		t.Error("unknown level accepted")
	}
	if err := c.Move(ctx(t), strings.Repeat("F", 40), "up"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown hash: %v", err)
	}
}

func TestGlobalAndCapabilities(t *testing.T) {
	for _, p := range profiles {
		t.Run(p.version, func(t *testing.T) {
			f := newFake(t, p)
			c := f.client(t)
			id, _ := c.Add(ctx(t), AddRequest{Magnet: magnet})
			stopped, _ := c.Add(ctx(t), AddRequest{Torrent: torrentFile, Paused: true})
			globals(t, f, c, id, stopped)
			capabilities(t, f, c, id)
		})
	}
}

func globals(t *testing.T, f *fakeRT, c *Client, id, stopped string) {
	t.Helper()
	must(t, "PauseAll", c.PauseAll(ctx(t)))
	if f.get(id).active || f.get(stopped).started {
		t.Error("PauseAll left a download active or started a stopped one")
	}
	st, err := c.Stats(ctx(t))
	must(t, "Stats", err)
	if !st.Paused || st.DownRate != 2048 || st.UpRate != 512 || st.FreeSpace != 1<<40 {
		t.Errorf("stats %+v", st)
	}
	must(t, "ResumeAll", c.ResumeAll(ctx(t)))
	if !f.get(id).active || f.get(stopped).started {
		t.Error("ResumeAll did not restore exactly the paused downloads")
	}
	must(t, "SetLimits", c.SetLimits(ctx(t), Limits{Down: 1 << 20, Up: 1 << 19}))
	l, err := c.Limits(ctx(t))
	if err != nil || l.Down != 1<<20 || l.Up != 1<<19 {
		t.Errorf("limits %+v %v", l, err)
	}
	if err := c.SetLimits(ctx(t), Limits{Down: -1}); KindOf(err) != KindConfig {
		t.Errorf("negative limit: %v", err)
	}
}

func capabilities(t *testing.T, f *fakeRT, c *Client, id string) {
	t.Helper()
	must(t, "CheckHash", c.CheckHash(ctx(t), id))
	err := c.Reannounce(ctx(t), id)
	switch {
	case f.prof.missing["d.tracker_announce"]:
		if !errors.Is(err, ErrUnsupported) {
			t.Errorf("reannounce on a build without it: %v", err)
		}
	case err != nil:
		t.Errorf("reannounce: %v", err)
	}
	trs, err := c.Trackers(ctx(t), id)
	must(t, "Trackers", err)
	if len(trs) != 1 || trs[0].URL != "https://tracker.example" || trs[0].Status != "working" || trs[0].Seeds != 10 {
		t.Errorf("trackers %+v", trs)
	}
	files, err := c.Files(ctx(t), id)
	must(t, "Files", err)
	if len(files) != 2 || files[0].Progress != 0.5 || files[1].Priority != 0 || files[0].Size != 1000 {
		t.Errorf("files %+v", files)
	}
	free, err := c.FreeSpace(ctx(t))
	if err != nil || free != 1<<40 {
		t.Errorf("free %d %v", free, err)
	}
	if _, err := c.Trackers(ctx(t), strings.Repeat("F", 40)); !errors.Is(err, ErrNotFound) {
		t.Errorf("trackers of unknown hash: %v", err)
	}
	if _, err := c.Files(ctx(t), strings.Repeat("F", 40)); !errors.Is(err, ErrNotFound) {
		t.Errorf("files of unknown hash: %v", err)
	}
}

func TestFreeSpaceUnknownAndStatsEmpty(t *testing.T) {
	f := newFake(t, v098)
	c := f.client(t)
	free, err := c.FreeSpace(ctx(t))
	if err != nil || free != -1 {
		t.Errorf("free %d %v", free, err)
	}
	st, err := c.Stats(ctx(t))
	if err != nil || st.Paused || st.FreeSpace != -1 {
		t.Errorf("stats %+v %v", st, err)
	}
}

func TestTrackerStatus(t *testing.T) {
	cases := map[string][]any{
		"disabled":      {"", int64(0), nil, nil, int64(1), int64(0)},
		"not_working":   {"", int64(1), nil, nil, int64(1), int64(2)},
		"working":       {"", int64(1), nil, nil, int64(3), int64(0)},
		"not_contacted": {"", int64(1), nil, nil, int64(0), int64(0)},
	}
	for want, r := range cases {
		if got := trackerStatus(r); got != want {
			t.Errorf("%v: %s, want %s", r, got, want)
		}
	}
	if TrackerOrigin("::bad") != "(unparsable tracker address)" || TrackerOrigin("udp://t.example:80/announce") != "udp://t.example:80" {
		t.Error("tracker origin")
	}
}

func TestAuthAndHTTPFailures(t *testing.T) {
	f := newFake(t, v098)
	c := f.client(t, func(cfg *Config) { cfg.Password = "wrong" })
	if _, err := c.Version(ctx(t)); KindOf(err) != KindAuth {
		t.Fatalf("want auth, got %v", err)
	}
	for status, kind := range map[int]Kind{403: KindAuth, 404: KindConfig, 500: KindServer, 302: KindServer} {
		f.status = status
		if _, err := f.client(t).Version(ctx(t)); KindOf(err) != kind {
			t.Errorf("HTTP %d: %v, want %s", status, err, kind)
		}
	}
	f.status = 0
	dead := f.client(t, func(cfg *Config) { cfg.URL = "http://127.0.0.1:1/RPC2" })
	if _, err := dead.Version(ctx(t)); KindOf(err) != KindUnreachable {
		t.Errorf("unreachable: %v", err)
	}
}

func TestTimeout(t *testing.T) {
	slow := doerFunc(func(ctx context.Context) (int, []byte, error) {
		<-ctx.Done()
		return 0, nil, ctx.Err()
	})
	c, _ := New(slow, Config{URL: "http://rt.invalid", Timeout: 10 * time.Millisecond})
	if _, err := c.Version(ctx(t)); KindOf(err) != KindTimeout {
		t.Fatalf("want timeout, got %v", err)
	}
}

func TestFaults(t *testing.T) {
	f := newFake(t, v098)
	c := f.client(t)
	_, err := c.Call(ctx(t), "d.state", strings.Repeat("F", 40))
	var fault *Fault
	if !errors.Is(err, ErrNotFound) || !errors.As(err, &fault) || fault.Code != -501 {
		t.Errorf("unknown hash: %v", err)
	}
	f.raw["system.client_version"] = EncodeFault(-1, "boom")
	if _, err := c.Call(ctx(t), "system.client_version"); KindOf(err) != KindServer || !strings.Contains(err.Error(), "boom") {
		t.Errorf("fault: %v", err)
	}
	if _, err := c.Call(ctx(t), "no.such"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("unknown method: %v", err)
	}
	if _, err := c.Call(ctx(t), "x", func() {}); KindOf(err) != KindConfig {
		t.Errorf("unencodable param: %v", err)
	}
	// A fault inside a batch ends it with that call's error.
	if _, err := c.Batch(ctx(t), []Request{{Method: "system.client_version"}, {Method: "no.such"}}); err == nil {
		t.Error("batch fault ignored")
	}
	f.raw["system.multicall"] = mustEncode(t, []any{int64(1)})
	if _, err := c.Batch(ctx(t), []Request{{Method: "a"}, {Method: "b"}}); KindOf(err) != KindParse {
		t.Errorf("short batch answer: %v", err)
	}
}

func TestMalformedAnswers(t *testing.T) {
	f := newFake(t, v098)
	c := f.client(t)
	f.raw["d.multicall2"] = mustEncode(t, "not rows")
	if _, err := c.List(ctx(t), nil); KindOf(err) != KindParse {
		t.Errorf("list of a string: %v", err)
	}
	f.raw["d.multicall2"] = mustEncode(t, []any{[]any{"short"}})
	if _, err := c.List(ctx(t), nil); KindOf(err) != KindParse {
		t.Errorf("short row: %v", err)
	}
	f.raw["t.multicall"] = mustEncode(t, "x")
	f.raw["f.multicall"] = mustEncode(t, "x")
	if _, err := c.Trackers(ctx(t), "x"); KindOf(err) != KindParse {
		t.Errorf("trackers: %v", err)
	}
	if _, err := c.Files(ctx(t), "x"); KindOf(err) != KindParse {
		t.Errorf("files: %v", err)
	}
	f.raw["system.client_version"] = []byte("<html><body>login</body></html>")
	if _, err := c.Call(ctx(t), "system.client_version"); KindOf(err) != KindParse {
		t.Errorf("html page: %v", err)
	}
}

func TestPropagatesErrors(t *testing.T) {
	f := newFake(t, v098)
	c := f.client(t)
	f.status = 500
	h := strings.Repeat("A", 40)
	checks := map[string]error{
		"Add":        func() error { _, err := c.Add(ctx(t), AddRequest{Magnet: magnet}); return err }(),
		"Remove":     c.Remove(ctx(t), h, false),
		"Pause":      c.Pause(ctx(t), h),
		"Resume":     c.Resume(ctx(t), h),
		"List":       func() error { _, err := c.List(ctx(t), nil); return err }(),
		"Limits":     func() error { _, err := c.Limits(ctx(t)); return err }(),
		"Stats":      func() error { _, err := c.Stats(ctx(t)); return err }(),
		"FreeSpace":  func() error { _, err := c.FreeSpace(ctx(t)); return err }(),
		"Version":    func() error { _, err := c.Version(ctx(t)); return err }(),
		"Trackers":   func() error { _, err := c.Trackers(ctx(t), h); return err }(),
		"Files":      func() error { _, err := c.Files(ctx(t), h); return err }(),
		"Exists":     func() error { _, err := c.Exists(ctx(t), h); return err }(),
		"PauseAll":   c.PauseAll(ctx(t)),
		"MoveUp":     c.Move(ctx(t), h, "up"),
		"Sequential": func() error { _, err := c.sequential(ctx(t), []Request{{Method: "x"}}); return err }(),
	}
	for name, err := range checks {
		if KindOf(err) != KindServer {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestStatsPartialFailure covers Stats when the rates succeed and the disk
// read fails, and Remove when the erase probe fails.
func TestStatsPartialFailure(t *testing.T) {
	f := newFake(t, v098)
	c := f.client(t)
	f.raw["directory.default"] = EncodeFault(-1, "boom")
	if _, err := c.Stats(ctx(t)); KindOf(err) != KindServer {
		t.Errorf("stats: %v", err)
	}
	delete(f.raw, "directory.default")
	f.raw["d.multicall2"] = EncodeFault(-1, "boom")
	if _, err := c.FreeSpace(ctx(t)); KindOf(err) != KindServer {
		t.Errorf("free space: %v", err)
	}
	delete(f.raw, "d.multicall2")
	id, _ := c.Add(ctx(t), AddRequest{Magnet: magnet})
	f.raw["method.get"] = []byte("garbage")
	if err := c.Remove(ctx(t), id, true); KindOf(err) != KindParse {
		t.Errorf("remove with failing probe: %v", err)
	}
}

func TestNewValidation(t *testing.T) {
	d := httpDoer{}
	bad := []Config{{URL: "ftp://x"}, {URL: "http://"}, {URL: "http://u:p@host/RPC2"}, {URL: "http://h", LabelField: "custom5"}, {URL: "%zz"}}
	for _, cfg := range bad {
		if _, err := New(d, cfg); KindOf(err) != KindConfig {
			t.Errorf("New(%+v): %v", cfg, err)
		}
	}
	if _, err := New(nil, Config{URL: "http://h"}); err == nil {
		t.Error("nil doer accepted")
	}
	for raw, want := range map[string]string{"http://h": "http://h/RPC2", "https://h/": "https://h/RPC2", "http://h/rutorrent/RPC2": "http://h/rutorrent/RPC2"} {
		c, err := New(d, Config{URL: raw})
		if err != nil || c.Endpoint() != want {
			t.Errorf("%s → %v %v", raw, c, err)
		}
	}
}

func TestNoAuthHeaderWithoutCredentials(t *testing.T) {
	var got map[string]string
	d := recordingDoer{header: &got}
	c, _ := New(d, Config{URL: "http://h"})
	_, _ = c.Version(ctx(t))
	if _, ok := got["Authorization"]; ok || got["Content-Type"] != "text/xml" {
		t.Errorf("header %v", got)
	}
}

func TestErrorStrings(t *testing.T) {
	e := errorf(KindConfig, "x %d", 1)
	if e.Error() != "config: x 1" || KindOf(errors.New("plain")) != "" {
		t.Error(e.Error())
	}
	w := wrap(KindServer, "m", &Fault{Code: 1, Message: "f"})
	if w.Error() != "server_error: m: fault 1: f" || errors.Is(w, ErrNotFound) {
		t.Error(w.Error())
	}
}
