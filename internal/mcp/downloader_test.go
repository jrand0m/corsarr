package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/queue"
	"github.com/jrand0m/corsarr/internal/store"
)

func grabOf(c *calls, kind string, id int64, release string, confirm bool) (domain.Grab, automation.Decided, error) {
	c.add(fmt.Sprintf("%s.grab %d %s %v", kind, id, release, confirm))
	d := automation.Decided{Release: domain.Release{Title: "Rel." + release}}
	switch release {
	case "shadow":
		return domain.Grab{}, d, automation.ErrShadowed
	case "gone":
		return domain.Grab{}, d, automation.ErrUnknownRelease
	}
	return domain.Grab{Downloader: "qbit"}, d, nil
}

func (f *fakeMovieSearch) GrabRelease(_ context.Context, id int64, rel string, confirm bool) (domain.Grab, automation.Decided, error) {
	return grabOf(f.c, "movie", id, rel, confirm)
}

func (f *fakeTVSearch) GrabRelease(_ context.Context, id int64, rel string, confirm bool) (domain.Grab, automation.Decided, error) {
	return grabOf(f.c, "tv", id, rel, confirm)
}

func (f *fakeMusicSearch) GrabRelease(_ context.Context, id int64, rel string, confirm bool) (domain.Grab, automation.Decided, error) {
	return grabOf(f.c, "music", id, rel, confirm)
}

// fakeDownloaders answers per client key: "qbit" does everything, "sab"
// lacks the optional capabilities, and any write while shadow is set is
// recorded instead of sent.
type fakeDownloaders struct {
	c      *calls
	shadow bool
	filter queue.Filter
}

func (f *fakeDownloaders) act(name string, key domain.ObjectKey) error {
	switch {
	case f.shadow:
		return queue.ErrShadowed
	case key == "sab":
		return fmt.Errorf("%s: %w", name, plugin.ErrUnsupported)
	case key == "gone":
		return queue.ErrUnknownClient
	}
	f.c.add(name + " " + string(key))
	return nil
}

// keyOf maps download ids onto clients: 1 is on qbit, 2 on sab.
func keyOfDownload(id int64) domain.ObjectKey {
	return map[int64]domain.ObjectKey{1: "qbit", 2: "sab"}[id]
}

func (f *fakeDownloaders) Clients(context.Context) []queue.ClientView {
	return []queue.ClientView{{Key: "qbit", Type: "qbittorrent"}, {Key: "sab", Type: "sabnzbd"}}
}

func (f *fakeDownloaders) ListFiltered(_ context.Context, fl queue.Filter, _ int64, _ int) ([]store.StoredDownload, int64, error) {
	f.filter = fl
	return []store.StoredDownload{{Download: domain.Download{ID: 1, Title: "Show.S01"}}}, 7, nil
}

func (f *fakeDownloaders) Detail(_ context.Context, id int64) (queue.Detail, error) {
	if id != 1 {
		return queue.Detail{}, queue.ErrNotFound
	}
	return queue.Detail{StoredDownload: store.StoredDownload{Download: domain.Download{ID: 1, Title: "Show.S01", State: domain.StateDownloading, Progress: 0.25}}}, nil
}

func (f *fakeDownloaders) Pause(_ context.Context, id int64) error {
	return f.act("pause", keyOfDownload(id))
}
func (f *fakeDownloaders) Resume(_ context.Context, id int64) error {
	return f.act("resume", keyOfDownload(id))
}
func (f *fakeDownloaders) SetPriority(_ context.Context, id int64, p plugin.Priority) error {
	return f.act("priority-"+string(p), keyOfDownload(id))
}
func (f *fakeDownloaders) Recheck(_ context.Context, id int64) error {
	return f.act("recheck", keyOfDownload(id))
}
func (f *fakeDownloaders) Reannounce(_ context.Context, id int64) error {
	return f.act("reannounce", keyOfDownload(id))
}
func (f *fakeDownloaders) SetCategory(_ context.Context, id int64, c string) error {
	return f.act("category-"+c, keyOfDownload(id))
}
func (f *fakeDownloaders) Remove(_ context.Context, id int64, o queue.RemoveOptions) error {
	return f.act(fmt.Sprintf("remove-%v-%v-%v", o.DeleteFiles, o.Blocklist, o.Research), keyOfDownload(id))
}
func (f *fakeDownloaders) RetryImport(_ context.Context, id int64) error {
	return f.act("retry", keyOfDownload(id))
}
func (f *fakeDownloaders) PauseAll(_ context.Context, key domain.ObjectKey) error {
	return f.act("pauseall", key)
}
func (f *fakeDownloaders) ResumeAll(_ context.Context, key domain.ObjectKey) error {
	return f.act("resumeall", key)
}
func (f *fakeDownloaders) SetSpeedLimits(_ context.Context, key domain.ObjectKey, l plugin.SpeedLimits) error {
	return f.act(fmt.Sprintf("limits-%d-%d", l.DownKiB, l.UpKiB), key)
}
func (f *fakeDownloaders) SetAltSpeed(_ context.Context, key domain.ObjectKey, on *bool) (bool, error) {
	want := on == nil || *on
	return want, f.act(fmt.Sprintf("alt-%v", want), key)
}
func (f *fakeDownloaders) TestClient(_ context.Context, key domain.ObjectKey) error {
	if key == "sab" {
		return errBoom
	}
	return nil
}

func TestDownloaderReads(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "list_downloaders", nil, "2 download clients", "sabnzbd")
	h.ok(t, "get_queue", map[string]any{"downloader": "qbit", "kind": "tv", "states": []string{"downloading", "stalled"}}, `1 downloads (more: pass cursor "7")`)
	if f := h.dl.filter; f.Downloader != "qbit" || f.Kind != domain.KindTV || len(f.States) != 2 {
		t.Fatalf("%+v", f)
	}
	h.fails(t, "get_queue", map[string]any{"cursor": "x"}, "cursor")
	h.ok(t, "get_download", map[string]any{"id": 1}, "Show.S01: downloading, 25%")
	h.fails(t, "get_download", map[string]any{"id": 9}, "not found")
	h.ok(t, "test_downloader", map[string]any{"downloader": "qbit"}, "qbit: ok")
	h.fails(t, "test_downloader", map[string]any{"downloader": "sab"}, "boom")
	h.fails(t, "test_downloader", map[string]any{}, "downloader:")
	ro := h.s.NewSession(KeyRead)
	for _, name := range []string{"list_downloaders", "get_queue", "get_download", "test_downloader"} {
		if h.s.byName[name].Write {
			t.Errorf("%s is a read tool", name)
		}
	}
	if out := h.callOn(t, ro, "pause_all", map[string]any{"downloader": "qbit"}); !out.IsError {
		t.Fatal("control needs write scope")
	}
}

func TestDownloadAndClientActions(t *testing.T) {
	h := newHarness(t)
	for name, args := range map[string]map[string]any{
		"pause_download": {"id": 1}, "resume_download": {"id": 1}, "recheck_download": {"id": 1}, "reannounce_download": {"id": 1},
		"retry_import": {"id": 1}, "set_download_priority": {"id": 1, "priority": "top"}, "set_download_category": {"id": 1, "category": "tv"},
		"remove_download": {"id": 1, "deleteFiles": true, "blocklist": true, "searchAgain": true},
		"pause_all":       {"downloader": "qbit"}, "resume_all": {"downloader": "qbit"}, "set_speed_limits": {"downloader": "qbit", "downKiB": 1024},
		"toggle_alt_speed": {"downloader": "qbit"},
	} {
		out := h.ok(t, name, args, name+": done")
		if out.Data["sent"] != true {
			t.Errorf("%s: %v", name, out.Data)
		}
	}
	for _, c := range []string{"pause qbit", "priority-top qbit", "category-tv qbit", "remove-true-true-true qbit", "limits-1024-0 qbit", "alt-true qbit", "pauseall qbit"} {
		if !h.c.has(c) {
			t.Errorf("%s not sent: %v", c, h.c.list)
		}
	}
	h.fails(t, "pause_download", map[string]any{"id": 2}, "unsupported: pause: not supported by this client")
	h.fails(t, "pause_all", map[string]any{"downloader": "sab"}, "unsupported")
	h.fails(t, "pause_all", map[string]any{"downloader": "gone"}, "no downloader with this key")
	h.fails(t, "pause_download", map[string]any{}, "id:")
	h.fails(t, "pause_all", map[string]any{}, "downloader:")
	h.fails(t, "set_speed_limits", map[string]any{}, "downloader:")
	h.fails(t, "toggle_alt_speed", map[string]any{}, "downloader:")
	h.fails(t, "set_download_priority", map[string]any{"id": 1, "priority": "first"}, "priority: must be")
	h.fails(t, "toggle_alt_speed", map[string]any{"downloader": "sab", "on": false}, "unsupported")
	h.dl.shadow = true
	out := h.ok(t, "remove_download", map[string]any{"id": 1}, "recorded, not sent (shadow mode)")
	if out.Data["recorded"] != true || out.Data["sent"] != false {
		t.Fatal(out.Data)
	}
	if out := h.ok(t, "toggle_alt_speed", map[string]any{"downloader": "qbit", "on": false}); out.Data["on"] != false {
		t.Fatal(out.Data)
	}
	if !h.s.byName["remove_download"].Destructive || h.s.byName["toggle_alt_speed"].Idempotent {
		t.Fatal("hints")
	}
}

func TestGrabRelease(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "grab_release", map[string]any{"kind": "movie", "id": 2, "releaseId": "r1"}, "grabbed Rel.r1 on qbit")
	h.ok(t, "grab_release", map[string]any{"kind": "tv", "id": 1, "releaseId": "r2", "override": true}, "grabbed")
	h.ok(t, "grab_release", map[string]any{"kind": "music", "id": 100, "releaseId": "shadow"}, "recorded, not sent (shadow mode): Rel.shadow")
	if !h.c.has("tv.grab 1 r2 true") || !h.c.has("movie.grab 2 r1 false") {
		t.Fatal(h.c.list)
	}
	h.fails(t, "grab_release", map[string]any{"kind": "movie", "id": 2, "releaseId": "gone"}, "search again")
	h.fails(t, "grab_release", map[string]any{"kind": "x"}, "kind")
}

func TestDownloaderToolsNeedTheService(t *testing.T) {
	h := newHarness(t)
	d := h.s.d
	d.Downloaders = nil
	if s := New(d); s.byName["pause_all"] != nil || s.byName["find_media"] == nil {
		t.Fatal("without a downloader service only the other tools are listed")
	}
	if !strings.Contains(h.s.byName["grab_release"].Description, "search_releases") {
		t.Fatal("grab_release points at search_releases")
	}
}
