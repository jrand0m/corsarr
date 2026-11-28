package main

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/plugins/rtorrent/client"
	"github.com/jrand0m/corsarr/plugins/rtorrent/fakexmlrpc"
	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugintest"
)

const (
	hash   = "C0C0C0C0C0C0C0C0C0C0C0C0C0C0C0C0C0C0C0C0"
	magnet = "magnet:?xt=urn:btih:c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0&dn=Some.Show"
)

func start(t *testing.T, p fakexmlrpc.Profile, settings map[string]any) (*fakexmlrpc.Server, *plugintest.Harness) {
	t.Helper()
	fake := fakexmlrpc.New(p, fakexmlrpc.Options{Username: "rt", Password: "pw"})
	srv := httptest.NewServer(fake.Handler())
	t.Cleanup(srv.Close)
	s := map[string]any{"url": srv.URL + "/RPC2", "username": "rt", "timeout": "5s", "labelField": "custom1"}
	for k, v := range settings {
		s[k] = v
	}
	h := plugintest.New(plugintest.Host{Settings: s, Secrets: map[string]string{"password": "pw"}, Hosts: []string{"127.0.0.1"}})
	return fake, h
}

func call(t *testing.T, h *plugintest.Harness, op string, req, resp any) {
	t.Helper()
	if err := h.Call(op, req, resp); err != nil {
		t.Fatalf("%s: %v", op, err)
	}
}

func classOf(err error) string {
	var e *abi.Error
	if errors.As(err, &e) {
		return e.Class
	}
	return ""
}

func TestManifestPassesHostRules(t *testing.T) {
	b, err := manifest.JSON()
	if err != nil {
		t.Fatal(err)
	}
	m, err := wasm.ParseManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Settings.Fields) != len(client.Fields()) || m.Settings.Fields[4].Type != abi.TypeEnum {
		t.Errorf("%+v", m.Settings.Fields)
	}
}

func TestModuleAgainstEachProfile(t *testing.T) {
	for _, p := range fakexmlrpc.Profiles {
		t.Run(p.Name, func(t *testing.T) {
			fake, h := start(t, p, map[string]any{"pathMap": map[string]any{"/downloads": "/mnt/dl"}})
			caps, err := h.Capabilities()
			if err != nil || len(caps.Ops) != len(abi.DownloaderRequired)+14 || len(caps.Flags) != 1 || caps.Flags[0] != abi.FlagRemoveKeepsData {
				t.Fatalf("%d ops %v %v", len(caps.Ops), caps.Flags, err)
			}
			call(t, h, abi.OpTest, struct{}{}, nil)
			call(t, h, abi.OpDLEnsureCategory, abi.CategoryRequest{Category: "tv shows"}, nil)
			var added abi.AddResponse
			req := abi.AddRequest{Payload: abi.Payload{Kind: "magnet", URL: magnet}, Category: "tv shows", SavePath: "/mnt/dl/tv", Paused: true}
			call(t, h, abi.OpDLAdd, req, &added)
			var again abi.AddResponse
			call(t, h, abi.OpDLAdd, req, &again)
			if added.ID != hash || again.ID != hash || len(fake.Torrents()) != 1 {
				t.Fatalf("%q %q %d", added.ID, again.ID, len(fake.Torrents()))
			}
			if got := fake.Torrents()[0]; got.Dir != "/downloads/tv" || got.Custom["custom1"] != "tv%20shows" || got.Started {
				t.Errorf("loaded %+v", got)
			}
			var list abi.ListResponse
			call(t, h, abi.OpDLList, abi.ListRequest{Categories: []string{"tv shows"}}, &list)
			if len(list.Downloads) != 1 || list.Downloads[0].State != "queued" || list.Downloads[0].SavePath != "/mnt/dl/tv" {
				t.Fatalf("%+v", list)
			}
			controls(t, h, added.ID, p)
			removeWithData(t, h, fake, p)
		})
	}
}

func controls(t *testing.T, h *plugintest.Harness, id string, p fakexmlrpc.Profile) {
	t.Helper()
	for _, c := range []struct {
		op  string
		req any
	}{
		{abi.OpDLResume, abi.IDRequest{ID: id}}, {abi.OpDLPause, abi.IDRequest{ID: id}}, {abi.OpDLResume, abi.IDRequest{ID: id}},
		{abi.OpDLSetPriority, abi.PriorityRequest{ID: id, Priority: "top"}},
		{abi.OpDLSetPriority, abi.PriorityRequest{ID: id, Priority: "down"}},
		{abi.OpDLSetPriorityLevel, abi.PriorityRequest{ID: id, Priority: "force"}},
		{abi.OpDLRecheck, abi.IDRequest{ID: id}}, {abi.OpDLReannounce, abi.IDRequest{ID: id}},
		{abi.OpDLSetCategory, abi.CategoryRequest{ID: id, Category: "movies"}},
		{abi.OpDLPauseAll, struct{}{}}, {abi.OpDLResumeAll, struct{}{}},
		{abi.OpDLSetSpeedLimits, abi.SpeedLimits{DownKiB: 500, UpKiB: 50}}, {abi.OpDLSetAltSpeed, abi.Toggle{}},
	} {
		call(t, h, c.op, c.req, nil)
	}
	if err := h.Call(abi.OpDLSetAltSpeed, abi.Toggle{On: true}, nil); classOf(err) != abi.ClassUnsupported {
		t.Errorf("alt speed on: %v", err)
	}
	if err := h.Call(abi.OpDLSetPriorityLevel, abi.PriorityRequest{ID: id, Priority: "max"}, nil); classOf(err) != abi.ClassConfig {
		t.Errorf("unknown level: %v", err)
	}
	var files abi.FilesResponse
	call(t, h, abi.OpDLFiles, abi.IDRequest{ID: id}, &files)
	var trackers abi.TrackersResponse
	call(t, h, abi.OpDLTrackers, abi.IDRequest{ID: id}, &trackers)
	if len(files.Files) != 1 || len(trackers.Trackers) != 1 || trackers.Trackers[0].URL != "https://tracker.example" {
		t.Errorf("%+v %+v", files, trackers)
	}
	var limits abi.SpeedLimits
	call(t, h, abi.OpDLSpeedLimits, struct{}{}, &limits)
	var alt abi.Toggle
	call(t, h, abi.OpDLAltSpeed, struct{}{}, &alt)
	var stats abi.Stats
	call(t, h, abi.OpDLStats, struct{}{}, &stats)
	if limits.DownKiB != 500 || limits.UpKiB != 50 || alt.On || stats.FreeSpace == 0 || stats.Limits.DownKiB != 500 {
		t.Errorf("%+v %+v %+v", limits, alt, stats)
	}
	var res abi.ActionResult
	call(t, h, abi.OpAction, abi.ActionRequest{ID: "test"}, &res)
	want := map[bool]string{true: abi.OutcomeOK, false: abi.OutcomeWarn}[p.EraseData]
	if res.Outcome != want || !strings.Contains(res.Text, "Connected to rTorrent "+p.Version) {
		t.Errorf("%+v", res)
	}
}

// removeWithData: a front end's handler erases the data; without one the
// remove is refused as unsupported and nothing is removed.
func removeWithData(t *testing.T, h *plugintest.Harness, fake *fakexmlrpc.Server, p fakexmlrpc.Profile) {
	t.Helper()
	err := h.Call(abi.OpDLRemove, abi.RemoveRequest{ID: hash, DeleteFiles: true}, nil)
	if !p.EraseData {
		if classOf(err) != abi.ClassUnsupported || len(fake.Torrents()) != 1 {
			t.Fatalf("remove with data without handler: %v", err)
		}
		call(t, h, abi.OpDLRemove, abi.RemoveRequest{ID: hash}, nil)
	} else if err != nil || !fake.Erased()[0].ErasedData {
		t.Fatalf("remove with data: %v %+v", err, fake.Erased())
	}
	if len(fake.Torrents()) != 0 {
		t.Error("not removed")
	}
	call(t, h, abi.OpDLRemove, abi.RemoveRequest{ID: hash}, nil)
	if err := h.Call(abi.OpDLPause, abi.IDRequest{ID: hash}, nil); classOf(err) != abi.ClassNotFound {
		t.Errorf("removed download: %v", err)
	}
}

func TestStateFolding(t *testing.T) {
	cases := []struct {
		in    client.Torrent
		state string
		err   bool
	}{
		{client.Torrent{State: client.StateFailed, Message: "Disk full"}, "stalled", true},
		{client.Torrent{State: client.StateSeeding, ContentPath: "/d/x"}, "completed", false},
		{client.Torrent{State: client.StateChecking, Complete: true}, "completed", false},
		{client.Torrent{State: client.StateChecking}, "downloading", false},
		{client.Torrent{State: client.StatePaused}, "queued", false},
		{client.Torrent{State: "odd"}, "queued", false},
	}
	for _, c := range cases {
		s := snapshot(c.in)
		if s.State != c.state || (s.Error != "") != c.err {
			t.Errorf("%s: %+v", c.in.State, s)
		}
		if c.err && !strings.Contains(s.Error, "Disk full") {
			t.Errorf("reason lost: %q", s.Error)
		}
	}
}

func TestPayloadsAndFailures(t *testing.T) {
	fake, h := start(t, fakexmlrpc.V010, nil)
	torrent := []byte("d4:infod6:lengthi5e4:name1:x12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaaee")
	var added abi.AddResponse
	call(t, h, abi.OpDLAdd, abi.AddRequest{Payload: abi.Payload{Kind: "torrent", Content: torrent}}, &added)
	if len(added.ID) != 40 || !fake.Torrents()[0].Started {
		t.Errorf("%+v %+v", added, fake.Torrents())
	}
	if err := h.Call(abi.OpDLAdd, abi.AddRequest{Payload: abi.Payload{Kind: "torrent", URL: "https://indexer.example/t/1"}}, nil); classOf(err) != abi.ClassConfig {
		t.Errorf("bare url: %v", err)
	}
	plugintest.New(plugintest.Host{Settings: map[string]any{"url": "ftp://x"}, Secrets: map[string]string{"password": ""}})
	if err := h.Call(abi.OpTest, struct{}{}, nil); classOf(err) != abi.ClassConfig {
		t.Errorf("bad url: %v", err)
	}
	var res abi.ActionResult
	call(t, h, abi.OpAction, abi.ActionRequest{ID: "test"}, &res)
	if res.Outcome != abi.OutcomeFail {
		t.Errorf("%+v", res)
	}
	plugintest.New(plugintest.Host{Settings: map[string]any{"url": "http://elsewhere.test/RPC2"}, Secrets: map[string]string{"password": ""},
		Hosts: []string{"127.0.0.1"}})
	if err := h.Call(abi.OpTest, struct{}{}, nil); classOf(err) != abi.ClassDenied {
		t.Errorf("refused host: %v", err)
	}
	call(t, h, abi.OpAction, abi.ActionRequest{ID: "test"}, &res)
	if res.Outcome != abi.OutcomeFail {
		t.Errorf("%+v", res)
	}
	plugintest.New(plugintest.Host{Settings: map[string]any{"url": "http://x"}})
	if err := h.Call(abi.OpTest, struct{}{}, nil); classOf(err) != abi.ClassDenied {
		t.Errorf("undeclared secret: %v", err)
	}
	if classify(nil) != nil || classOf(classify(errors.New("x"))) != abi.ClassServerError {
		t.Error("classify")
	}
}
