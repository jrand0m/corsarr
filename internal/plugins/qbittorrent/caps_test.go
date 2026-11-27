package qbittorrent

import (
	"context"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
)

func TestGlobalPauseUsesTheVersionsVerb(t *testing.T) {
	for version, verbs := range map[string][2]string{"v4.6.2": {"torrents/pause", "torrents/resume"}, "v5.0.1": {"torrents/stop", "torrents/start"}} {
		f := newFake(t, version)
		c := newClient(t, f, nil)
		if err := c.PauseAll(t.Context()); err != nil || !f.called(verbs[0]) {
			t.Fatalf("%s pause all: %v", version, err)
		}
		if err := c.ResumeAll(t.Context()); err != nil || !f.called(verbs[1]) {
			t.Fatalf("%s resume all: %v", version, err)
		}
	}
}

func TestSpeedLimitsAndAltMode(t *testing.T) {
	f := newFake(t, "v5.0.1")
	c := newClient(t, f, nil)
	ctx := context.Background()
	if err := c.SetSpeedLimits(ctx, plugin.SpeedLimits{DownKiB: 500, UpKiB: 0}); err != nil {
		t.Fatal(err)
	}
	if got, err := c.SpeedLimits(ctx); err != nil || got != (plugin.SpeedLimits{DownKiB: 500}) || f.dlLimit != 500*1024 {
		t.Fatalf("limits %+v %v", got, err)
	}
	if err := c.SetSpeedLimits(ctx, plugin.SpeedLimits{DownKiB: -1}); plugin.ClassOf(err) != plugin.ClassConfig {
		t.Fatalf("negative limit: %v", err)
	}
	for _, on := range []bool{true, true, false} {
		if err := c.SetAltSpeed(ctx, on); err != nil {
			t.Fatal(err)
		}
		if got, err := c.AltSpeed(ctx); err != nil || got != on {
			t.Fatalf("alt %v: %v %v", on, got, err)
		}
	}
}

func TestRecheckReannounceAndCategory(t *testing.T) {
	f := newFake(t, "v5.0.1")
	c := newClient(t, f, nil)
	ctx := context.Background()
	f.put(magnetHex, map[string]any{"name": "m", "state": "downloading", "category": "movies"})
	if err := c.Recheck(ctx, magnetHex); err != nil || !f.called("torrents/recheck="+magnetHex) {
		t.Fatal(err)
	}
	if err := c.Reannounce(ctx, magnetHex); err != nil || !f.called("torrents/reannounce="+magnetHex) {
		t.Fatal(err)
	}
	if err := c.SetCategory(ctx, magnetHex, "nope"); plugin.ClassOf(err) != plugin.ClassConfig {
		t.Fatalf("unknown category: %v", err)
	}
	f.categories["tv"] = ""
	if err := c.SetCategory(ctx, magnetHex, "tv"); err != nil || f.torrents[magnetHex]["category"] != "tv" {
		t.Fatal(err)
	}
}

func TestStatsAndTrackers(t *testing.T) {
	f := newFake(t, "v5.0.1")
	c := newClient(t, f, nil)
	ctx := context.Background()
	f.dlLimit = 1024 * 100
	f.put(magnetHex, map[string]any{"state": "stoppedDL"})
	st, err := c.Stats(ctx)
	if err != nil || st.DownSpeed != 2048 || st.UpSpeed != 1024 || st.FreeSpace != 5<<30 || !st.Paused || st.Limits.DownKiB != 100 {
		t.Fatalf("%+v %v", st, err)
	}
	f.put("ffff", map[string]any{"state": "downloading"})
	if st, _ := c.Stats(ctx); st.Paused {
		t.Fatal("one running torrent means not paused")
	}
	f.trackers = map[string][]map[string]any{magnetHex: {
		{"url": "** [DHT] **", "status": 2},
		{"url": "https://tracker.example/announce/SECRETPASSKEY?x=1", "status": 4, "msg": "unregistered", "num_seeds": 3}}}
	tr, err := c.Trackers(ctx, magnetHex)
	if err != nil || len(tr) != 1 || tr[0].URL != "https://tracker.example" || tr[0].Status != "not_working" || tr[0].Seeds != 3 {
		t.Fatalf("%+v %v", tr, err)
	}
	if trackerOrigin("::") == "" {
		t.Fatal("unparsable address must still say something")
	}
}
