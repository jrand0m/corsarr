package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/plugins/transmission/client"
	"github.com/jrand0m/corsarr/plugins/transmission/fakerpc"
	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugintest"
)

const magnet = "magnet:?xt=urn:btih:c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0&dn=x"

func start(t *testing.T, v fakerpc.Version, settings map[string]any) (*fakerpc.Server, *plugintest.Harness) {
	t.Helper()
	fake := fakerpc.New(v, fakerpc.Options{Username: "u", Password: "p"})
	srv := httptest.NewServer(fake.Handler())
	t.Cleanup(srv.Close)
	s := map[string]any{"url": srv.URL, "username": "u", "timeout": "5s"}
	for k, val := range settings {
		s[k] = val
	}
	h := plugintest.New(plugintest.Host{Settings: s, Secrets: map[string]string{"password": "p"}, Hosts: []string{"127.0.0.1"}})
	return fake, h
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
	if m.Permissions.HTTP.Hosts[0] != "$setting:url" || len(m.Settings.Fields) != len(client.Settings()) {
		t.Errorf("%+v", m.Permissions)
	}
}

func TestModuleAgainstEachVersion(t *testing.T) {
	for _, v := range fakerpc.Versions {
		t.Run(v.Name, func(t *testing.T) {
			fake, h := start(t, v, nil)
			caps, err := h.Capabilities()
			if err != nil || len(caps.Ops) != len(abi.DownloaderRequired)+14 || len(caps.Flags) != 0 {
				t.Fatalf("%d ops %v %v", len(caps.Ops), caps.Flags, err)
			}
			call(t, h, abi.OpTest, struct{}{}, nil)
			call(t, h, abi.OpDLEnsureCategory, abi.CategoryRequest{Category: "tv"}, nil)
			var added abi.AddResponse
			call(t, h, abi.OpDLAdd, abi.AddRequest{Payload: abi.Payload{Kind: "magnet", URL: magnet}, Category: "tv", Paused: true}, &added)
			if added.ID != "c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0" {
				t.Fatalf("id %q", added.ID)
			}
			var again abi.AddResponse
			call(t, h, abi.OpDLAdd, abi.AddRequest{Payload: abi.Payload{Kind: "magnet", URL: magnet}, Category: "tv"}, &again)
			if again.ID != added.ID || len(fake.Torrents()) != 1 {
				t.Errorf("add is not idempotent: %q %d", again.ID, len(fake.Torrents()))
			}
			var list abi.ListResponse
			call(t, h, abi.OpDLList, abi.ListRequest{Categories: []string{"tv"}}, &list)
			if len(list.Downloads) != 1 || list.Downloads[0].Category != "tv" || list.Downloads[0].State != "queued" {
				t.Fatalf("%+v", list)
			}
			controls(t, h, added.ID)
			call(t, h, abi.OpDLRemove, abi.RemoveRequest{ID: added.ID, DeleteFiles: true}, nil)
			if len(fake.Torrents()) != 0 {
				t.Error("not removed")
			}
		})
	}
}

func controls(t *testing.T, h *plugintest.Harness, id string) {
	t.Helper()
	for _, c := range []struct {
		op  string
		req any
	}{
		{abi.OpDLPause, abi.IDRequest{ID: id}}, {abi.OpDLResume, abi.IDRequest{ID: id}},
		{abi.OpDLSetPriority, abi.PriorityRequest{ID: id, Priority: "top"}},
		{abi.OpDLSetPriorityLevel, abi.PriorityRequest{ID: id, Priority: "high"}},
		{abi.OpDLRecheck, abi.IDRequest{ID: id}}, {abi.OpDLReannounce, abi.IDRequest{ID: id}},
		{abi.OpDLSetCategory, abi.CategoryRequest{ID: id, Category: "movies"}},
		{abi.OpDLSetCategory, abi.CategoryRequest{ID: id, Category: "tv"}},
		{abi.OpDLPauseAll, struct{}{}}, {abi.OpDLResumeAll, struct{}{}},
		{abi.OpDLSetSpeedLimits, abi.SpeedLimits{DownKiB: 500}}, {abi.OpDLSetAltSpeed, abi.Toggle{On: true}},
	} {
		call(t, h, c.op, c.req, nil)
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
	if limits.DownKiB != 500 || !alt.On || stats.FreeSpace == 0 || !stats.AltSpeed {
		t.Errorf("%+v %+v %+v", limits, alt, stats)
	}
	var res abi.ActionResult
	call(t, h, abi.OpAction, abi.ActionRequest{ID: "test"}, &res)
	if res.Outcome != abi.OutcomeOK || !strings.Contains(res.Text, "Connected to Transmission") {
		t.Errorf("%+v", res)
	}
}

func call(t *testing.T, h *plugintest.Harness, op string, req, resp any) {
	t.Helper()
	if err := h.Call(op, req, resp); err != nil {
		t.Fatalf("%s: %v", op, err)
	}
}

func TestStatesAndErrors(t *testing.T) {
	fake, h := start(t, fakerpc.V406, map[string]any{"categoryDirs": map[string]any{"movies": "/downloads/movies"}})
	fake.Add(
		fakerpc.Torrent{Hash: strings.Repeat("a", 40), Name: "seeding", Dir: "/downloads/movies", Status: fakerpc.StatusSeed, Size: 10},
		fakerpc.Torrent{Hash: strings.Repeat("b", 40), Name: "full disk", Dir: "/downloads", Labels: []string{"movies"},
			Status: fakerpc.StatusStopped, Error: 3, ErrorString: "No space left on device", Size: 10, Left: 5},
	)
	var list abi.ListResponse
	call(t, h, abi.OpDLList, abi.ListRequest{Categories: []string{"movies"}}, &list)
	if len(list.Downloads) != 2 {
		t.Fatalf("%+v", list)
	}
	seed, stalled := list.Downloads[0], list.Downloads[1]
	if seed.State != "completed" || seed.Category != "movies" || !slices.Equal(seed.Files, []string{"/downloads/movies/seeding"}) {
		t.Errorf("seeding: %+v", seed)
	}
	if stalled.State != "stalled" || stalled.Error != client.LocalErrorHint {
		t.Errorf("local error: %+v", stalled)
	}
	err := h.Call(abi.OpDLFiles, abi.IDRequest{ID: strings.Repeat("f", 40)}, nil)
	var e *abi.Error
	if !errors.As(err, &e) || e.Class != abi.ClassNotFound {
		t.Errorf("missing torrent: %v", err)
	}
}

func TestSettingsAndHostFailures(t *testing.T) {
	_, h := start(t, fakerpc.V300, nil)
	var e *abi.Error
	plugintest.New(plugintest.Host{Settings: map[string]any{"url": "nope"}, Secrets: map[string]string{"password": ""}})
	if err := h.Call(abi.OpTest, struct{}{}, nil); !errors.As(err, &e) || e.Class != abi.ClassConfig {
		t.Errorf("bad url: %v", err)
	}
	plugintest.New(plugintest.Host{Settings: map[string]any{"url": "http://elsewhere.test:9091"}, Secrets: map[string]string{"password": ""},
		Hosts: []string{"127.0.0.1"}})
	if err := h.Call(abi.OpTest, struct{}{}, nil); !errors.As(err, &e) || e.Class != abi.ClassDenied {
		t.Errorf("refused host: %v", err)
	}
	var res abi.ActionResult
	call(t, h, abi.OpAction, abi.ActionRequest{ID: "test"}, &res)
	if res.Outcome != abi.OutcomeFail {
		t.Errorf("%+v", res)
	}
	plugintest.New(plugintest.Host{Settings: map[string]any{"url": "http://x", "pathMap": 3}})
	if err := h.Call(abi.OpTest, struct{}{}, nil); err == nil {
		t.Error("undeclared secret or bad map passed")
	}
	call(t, h, abi.OpAction, abi.ActionRequest{ID: "test"}, &res)
	if res.Outcome != abi.OutcomeFail {
		t.Errorf("%+v", res)
	}
}

func TestClassify(t *testing.T) {
	if classify(nil) != nil {
		t.Error("nil")
	}
	for err, want := range map[error]string{
		errors.New("plain"):                              abi.ClassServerError,
		client.ErrUnsupported:                            abi.ClassUnsupported,
		&client.Error{Class: client.ClassAuth}:           abi.ClassAuth,
		&client.Error{Err: &abi.Error{Class: "timeout"}}: abi.ClassTimeout,
	} {
		var e *abi.Error
		if !errors.As(classify(err), &e) || e.Class != want {
			t.Errorf("%v: %v", err, classify(err))
		}
	}
}

// wrapped serves the fake behind a handler that counts session handshakes
// and fails free-space requests when told to.
func wrapped(t *testing.T, failFree bool) (*plugintest.Harness, func() int) {
	t.Helper()
	fake := fakerpc.New(fakerpc.Versions[len(fakerpc.Versions)-1], fakerpc.Options{})
	var mu sync.Mutex
	handshakes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-Transmission-Session-Id") == "" {
			mu.Lock()
			handshakes++
			mu.Unlock()
		} else if failFree && strings.Contains(string(body), `"free-space"`) {
			_, _ = w.Write([]byte(`{"result":"no such directory","arguments":{}}`))
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		fake.Handler().ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	h := plugintest.New(plugintest.Host{Settings: map[string]any{"url": srv.URL + fakerpc.Path, "timeout": "5s"},
		Secrets: map[string]string{"password": ""}, Hosts: []string{"127.0.0.1"}})
	return h, func() int { mu.Lock(); defer mu.Unlock(); return handshakes }
}

func TestTheSessionIsKeptBetweenCalls(t *testing.T) {
	h, handshakes := wrapped(t, false)
	for range 3 {
		call(t, h, abi.OpTest, struct{}{}, nil)
	}
	if n := handshakes(); n != 1 {
		t.Fatalf("%d session handshakes for three calls", n)
	}
}

func TestUnknownFreeSpaceReportsZero(t *testing.T) {
	h, _ := wrapped(t, true)
	var stats abi.Stats
	call(t, h, abi.OpDLStats, struct{}{}, &stats)
	if stats.FreeSpace != 0 || stats.DownSpeed != 2048 {
		t.Fatalf("%+v", stats)
	}
}
