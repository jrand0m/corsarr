package client

import (
	"slices"
	"testing"
)

func TestStateMapping(t *testing.T) {
	cases := []struct {
		name      string
		rt        rawTorrent
		state     State
		queue     State
		withError bool
	}{
		{"stopped incomplete", rawTorrent{Status: statusStopped, SizeWhenDone: 10, LeftUntilDone: 5, PercentDone: .5}, StatePaused, StateQueued, false},
		{"stopped complete", rawTorrent{Status: statusStopped, SizeWhenDone: 10, PercentDone: 1}, StateCompleted, StateCompleted, false},
		{"check wait incomplete", rawTorrent{Status: statusCheckWait, SizeWhenDone: 10, LeftUntilDone: 10}, StateChecking, StateDownloading, false},
		{"checking complete", rawTorrent{Status: statusCheck, SizeWhenDone: 10, PercentDone: 1}, StateChecking, StateCompleted, false},
		{"download wait", rawTorrent{Status: statusDownloadWait, SizeWhenDone: 10, LeftUntilDone: 10}, StateQueued, StateQueued, false},
		{"downloading", rawTorrent{Status: statusDownload, SizeWhenDone: 10, LeftUntilDone: 4, RateDownload: 100, PeersSendingToUs: 2}, StateDownloading, StateDownloading, false},
		{"no peers", rawTorrent{Status: statusDownload, SizeWhenDone: 10, LeftUntilDone: 4}, StateStalled, StateStalled, false},
		{"seed wait", rawTorrent{Status: statusSeedWait, SizeWhenDone: 10, PercentDone: 1}, StateCompleted, StateCompleted, false},
		{"seeding", rawTorrent{Status: statusSeed, SizeWhenDone: 10, PercentDone: 1}, StateSeeding, StateCompleted, false},
		{"local error", rawTorrent{Status: statusDownload, Error: errLocal, ErrorString: "No space left on device", RateDownload: 9, PeersSendingToUs: 1}, StateStalled, StateStalled, true},
		{"tracker error keeps state", rawTorrent{Status: statusDownload, Error: errTrackerError, ErrorString: "unregistered torrent", RateDownload: 9, PeersSendingToUs: 1, SizeWhenDone: 10, LeftUntilDone: 2}, StateDownloading, StateDownloading, false},
		{"tracker warning keeps state", rawTorrent{Status: statusSeed, Error: errTrackerWarning, PercentDone: 1}, StateSeeding, StateCompleted, false},
		{"unknown status", rawTorrent{Status: 42, SizeWhenDone: 10, LeftUntilDone: 10}, StateQueued, StateQueued, false},
	}
	c := &Client{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.torrent(tc.rt, "")
			if got.State != tc.state || got.QueueState != tc.queue {
				t.Errorf("state %s/%s, want %s/%s", got.State, got.QueueState, tc.state, tc.queue)
			}
			if (got.Error != "") != tc.withError || got.ErrorString != tc.rt.ErrorString || got.ErrorCode != tc.rt.Error {
				t.Errorf("error %q / %q", got.Error, got.ErrorString)
			}
		})
	}
	if QueueState(StateFailed, false) != StateFailed {
		t.Error("failed must stay failed")
	}
}

func TestTorrentFieldsAndPaths(t *testing.T) {
	f := newFake(t, "4.0.6 (38c164933e)", 17)
	f.add(&fakeTorrent{hash: "ABCDEF" + magnetHex[6:], name: "Show.S01", dir: "/data/tv", labels: []string{"tv"},
		status: statusSeed, percent: 1, size: 2000})
	c := f.client(t, Config{PathMap: map[string]string{"/data": "/mnt/media"}})
	list, err := c.List(tctx(t), nil)
	must(t, "List", err)
	got := list[0]
	if got.ID != "abcdef"+magnetHex[6:] || got.Category != "tv" || got.SavePath != "/mnt/media/tv" ||
		got.ContentPath != "/mnt/media/tv/Show.S01" || !slices.Equal(got.Files, []string{"/mnt/media/tv/Show.S01"}) ||
		got.Size != 2000 || got.Downloaded != 2000 || got.Ratio != 0 || got.ETASeconds != 0 || got.LastActivity != 1700000000 {
		t.Errorf("torrent = %+v", got)
	}
}

func TestCategoryFromDirectoryAndFirstLabel(t *testing.T) {
	f := newFake(t, "3.00 (bb6b5a062e)", 16)
	f.add(
		&fakeTorrent{hash: "1111111111111111111111111111111111111111", name: "a", dir: "/downloads/movies/"},
		&fakeTorrent{hash: "2222222222222222222222222222222222222222", name: "b", dir: "/downloads", labels: []string{"other", "x"}},
		&fakeTorrent{hash: "3333333333333333333333333333333333333333", name: "c", dir: "/downloads"},
	)
	c := f.client(t, Config{CategoryDirs: map[string]string{"movies": "/downloads/movies", "tv": "/downloads/tv"}})
	all, _ := c.List(tctx(t), nil)
	cats := []string{}
	for _, tr := range all {
		cats = append(cats, tr.Category)
	}
	if !slices.Equal(cats, []string{"movies", "other", ""}) {
		t.Errorf("categories = %v", cats)
	}
	movies, _ := c.List(tctx(t), []string{"movies"})
	if len(movies) != 1 || movies[0].Name != "a" {
		t.Errorf("movies = %+v", movies)
	}
}

func TestFilesAndTrackers(t *testing.T) {
	f := newFake(t, "4.0.6 (38c164933e)", 17)
	f.add(&fakeTorrent{hash: magnetHex, name: "x",
		files: []map[string]any{{"name": "x/a.mkv", "length": 700}, {"name": "x/b.nfo", "length": 3}},
		trackers: []map[string]any{
			{"announce": "https://tracker.example/announce/PASSKEY123?uid=7", "hasAnnounced": true, "lastAnnounceSucceeded": true, "seederCount": 12, "leecherCount": 3},
			{"announce": "udp://open.example:1337/announce", "hasAnnounced": true, "lastAnnounceSucceeded": false,
				"lastAnnounceResult": "Could not connect to http://open.example:1337/announce?passkey=abc", "seederCount": -1, "leecherCount": -1},
			{"announce": "http://slow.example/a/KEY", "announceState": 3},
			{"announce": "http://new.example/a/KEY"},
			{"announce": "::not a url"},
		}})
	c := f.client(t, Config{})
	files, err := c.Files(tctx(t), magnetHex)
	must(t, "Files", err)
	if !slices.Equal(files, []FileEntry{{"x/a.mkv", 700}, {"x/b.nfo", 3}}) {
		t.Errorf("files = %v", files)
	}
	trs, err := c.Trackers(tctx(t), magnetHex)
	must(t, "Trackers", err)
	want := []Tracker{
		{URL: "https://tracker.example", Status: "working", Seeds: 12, Peers: 3},
		{URL: "udp://open.example:1337", Status: "not_working", Message: "Could not connect to http://open.example:1337"},
		{URL: "http://slow.example", Status: "updating"},
		{URL: "http://new.example", Status: "not_contacted"},
		{URL: "(unparsable tracker address)", Status: "not_contacted"},
	}
	if !slices.Equal(trs, want) {
		t.Errorf("trackers =\n%+v\nwant\n%+v", trs, want)
	}
	if _, err := c.Trackers(tctx(t), "ffff"); err == nil {
		t.Error("unknown torrent listed trackers")
	}
	if ok, err := c.Exists(tctx(t), "ffff"); ok || err != nil {
		t.Errorf("exists unknown: %v %v", ok, err)
	}
	if ok, err := c.Exists(tctx(t), magnetHex); !ok || err != nil {
		t.Errorf("exists: %v %v", ok, err)
	}
}

func TestSpeedLimitsAndAltSpeed(t *testing.T) {
	f := newFake(t, "4.0.6 (38c164933e)", 17)
	c := f.client(t, Config{})
	l, err := c.SpeedLimits(tctx(t))
	must(t, "SpeedLimits", err)
	if l != (SpeedLimits{DownKiB: 976, UpKiB: 0}) { // 1000 kB/s with kB = 1000 bytes; upload limit off
		t.Errorf("limits = %+v", l)
	}
	must(t, "SetSpeedLimits", c.SetSpeedLimits(tctx(t), SpeedLimits{DownKiB: 0, UpKiB: 100}))
	args := f.args("session-set")
	if args["speed-limit-down-enabled"] != false || args["speed-limit-up-enabled"] != true || args["speed-limit-up"] != float64(102) {
		t.Errorf("session-set = %v", args)
	}
	if _, ok := args["speed-limit-down"]; ok {
		t.Error("an unlimited download sent a value")
	}
	if err := c.SetSpeedLimits(tctx(t), SpeedLimits{DownKiB: -1}); ClassOf(err) != ClassConfig {
		t.Errorf("negative: %v", err)
	}
	must(t, "SetAltSpeed", c.SetAltSpeed(tctx(t), true))
	on, err := c.AltSpeed(tctx(t))
	if err != nil || !on {
		t.Errorf("alt speed = %v %v", on, err)
	}
	if fromKiB(1, 1000) != 1 || fromKiB(1, 1<<20) != 1 || toKiB(1024, 1024) != 1024 {
		t.Error("unit conversion")
	}
}

func TestStatsAndFreeSpace(t *testing.T) {
	f := newFake(t, "4.0.6 (38c164933e)", 17)
	f.add(&fakeTorrent{hash: magnetHex, name: "x", status: statusDownload})
	c := f.client(t, Config{PathMap: map[string]string{"/downloads": "/mnt/dl"}})
	st, err := c.Stats(tctx(t))
	must(t, "Stats", err)
	want := Status{DownSpeed: 2048, UpSpeed: 512, FreeSpace: 500 << 30, Limits: SpeedLimits{DownKiB: 976}, Torrents: 1}
	if st != want {
		t.Errorf("stats = %+v", st)
	}
	res, err := c.Test(tctx(t))
	must(t, "Test", err)
	if res.RPCVersion != 17 || res.DownloadDir != "/mnt/dl" || res.FreeSpace != 500<<30 || res.Version == "" {
		t.Errorf("test = %+v", res)
	}
	if _, err := c.FreeSpace(tctx(t), "/mnt/dl/tv"); err != nil || f.args("free-space")["path"] != "/downloads/tv" {
		t.Errorf("free space path %v (%v)", f.args("free-space")["path"], err)
	}
	if _, err := c.FreeSpace(tctx(t), ""); err != nil || f.args("free-space")["path"] != "/downloads" {
		t.Errorf("default free space path %v (%v)", f.args("free-space")["path"], err)
	}
	f.mu.Lock()
	f.stats = map[string]any{"torrentCount": 0, "pausedTorrentCount": 0}
	f.mu.Unlock()
	if st, _ := c.Stats(tctx(t)); st.Paused {
		t.Error("an empty client counts as paused")
	}
}

func TestGlobalPauseSendsNoIDs(t *testing.T) {
	f := newFake(t, "3.00 (bb6b5a062e)", 16)
	c := f.client(t, Config{})
	must(t, "PauseAll", c.PauseAll(tctx(t)))
	must(t, "ResumeAll", c.ResumeAll(tctx(t)))
	if f.args("torrent-stop") != nil || f.args("torrent-start") != nil {
		t.Errorf("ids sent: %v %v", f.args("torrent-stop"), f.args("torrent-start"))
	}
}
