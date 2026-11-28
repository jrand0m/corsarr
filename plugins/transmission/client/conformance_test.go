package client

import (
	"context"
	"testing"
	"time"
)

const magnetHex = "0123456789abcdef0123456789abcdef01234567"

var torrentBytes = []byte("d4:infod4:name4:demo6:lengthi1000eee")

func tctx(t *testing.T) context.Context {
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

// downloaderLifecycle mirrors the host's downloader conformance checks: test,
// ensure category twice, idempotent add, listed in its category, pause,
// resume, every queue move, remove, gone from the list.
func downloaderLifecycle(t *testing.T, c *Client, add AddRequest) {
	t.Helper()
	_, err := c.Test(tctx(t))
	must(t, "Test", err)
	for range 2 {
		must(t, "EnsureCategory", c.EnsureCategory(tctx(t), add.Category, add.SavePath))
	}
	id, err := c.Add(tctx(t), add)
	must(t, "Add", err)
	if id == "" {
		t.Fatal("Add returned an empty id")
	}
	if again, err := c.Add(tctx(t), add); err != nil || again != id {
		t.Errorf("Add is not idempotent: %q then %q (%v)", id, again, err)
	}
	if !listed(t, c, add.Category, id) {
		t.Fatalf("torrent %s missing from List(%q)", id, add.Category)
	}
	must(t, "Pause", c.Pause(tctx(t), id))
	must(t, "Resume", c.Resume(tctx(t), id))
	for _, p := range []Priority{PrioTop, PrioUp, PrioDown, PrioBottom} {
		must(t, "SetPriority "+string(p), c.SetPriority(tctx(t), id, p))
	}
	must(t, "Remove", c.Remove(tctx(t), id, true))
	if listed(t, c, add.Category, id) {
		t.Errorf("torrent %s still listed after Remove", id)
	}
	must(t, "Remove again", c.Remove(tctx(t), id, true))
}

func listed(t *testing.T, c *Client, category, id string) bool {
	t.Helper()
	list, err := c.List(tctx(t), []string{category})
	must(t, "List", err)
	for _, tr := range list {
		if tr.ID == id {
			return true
		}
	}
	return false
}

func TestConformance(t *testing.T) {
	payloads := []struct {
		name string
		add  AddRequest
	}{
		{"magnet", AddRequest{URL: "magnet:?xt=urn:btih:" + magnetHex + "&dn=x", Category: "movies", SavePath: "/downloads/movies"}},
		{"torrent bytes", AddRequest{Torrent: torrentBytes, Category: "tv"}},
		{"url", AddRequest{URL: "https://example.invalid/file.torrent", Category: "music", SavePath: "/downloads/music"}},
	}
	for _, v := range fakeVersions {
		for _, p := range payloads {
			t.Run(v.name+"/"+p.name, func(t *testing.T) {
				f := newFake(t, v.version, v.rpc)
				downloaderLifecycle(t, f.client(t, Config{}), p.add)
			})
		}
	}
}

// Releases before labels recognise a category by its download directory.
func TestConformanceWithoutLabels(t *testing.T) {
	f := newFake(t, "2.94 (d8e60ee44f)", 15)
	c := f.client(t, Config{CategoryDirs: map[string]string{"tv": "/downloads/tv"}})
	downloaderLifecycle(t, c, AddRequest{URL: "magnet:?xt=urn:btih:" + magnetHex, Category: "tv"})
	downloaderLifecycle(t, c, AddRequest{Torrent: torrentBytes, Category: "movies", SavePath: "/downloads/movies"})
}

func TestConformanceCapabilities(t *testing.T) {
	for _, v := range fakeVersions {
		t.Run(v.name, func(t *testing.T) {
			f := newFake(t, v.version, v.rpc)
			c := f.client(t, Config{})
			id, err := c.Add(tctx(t), AddRequest{URL: "magnet:?xt=urn:btih:" + magnetHex, Category: "movies"})
			must(t, "Add", err)
			for _, l := range []Priority{PrioLow, PrioNormal, PrioHigh, PrioForce} {
				must(t, "SetPriorityLevel "+string(l), c.SetPriorityLevel(tctx(t), id, l))
			}
			must(t, "Recheck", c.Recheck(tctx(t), id))
			must(t, "Reannounce", c.Reannounce(tctx(t), id))
			must(t, "SetCategory", c.SetCategory(tctx(t), id, "tv"))
			if !listed(t, c, "tv", id) || listed(t, c, "movies", id) {
				t.Error("SetCategory did not move the torrent")
			}
			must(t, "PauseAll", c.PauseAll(tctx(t)))
			st, err := c.Stats(tctx(t))
			must(t, "Stats", err)
			if !st.Paused {
				t.Errorf("stats after PauseAll: %+v", st)
			}
			must(t, "ResumeAll", c.ResumeAll(tctx(t)))
			_, err = c.Files(tctx(t), id)
			must(t, "Files", err)
			_, err = c.Trackers(tctx(t), id)
			must(t, "Trackers", err)
		})
	}
}
