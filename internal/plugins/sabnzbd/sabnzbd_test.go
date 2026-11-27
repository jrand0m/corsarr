package sabnzbd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
)

// fakeSAB is a stateful in-memory SABnzbd JSON API.
type fakeSAB struct {
	srv *httptest.Server
	key string

	mu      sync.Mutex
	queue   []map[string]any
	history []map[string]any
	cats    []string
	paused  bool
	limit   string
	locked  bool // configuration locked: set_config fails
	calls   []string
	next    int
}

func newFake(t *testing.T) *fakeSAB {
	t.Helper()
	f := &fakeSAB{key: "k", cats: []string{"*", "movies", "tv"}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSAB) serve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("apikey") != f.key {
		_, _ = w.Write([]byte(`{"status":false,"error":"API Key Incorrect"}`))
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, q.Get("mode")+":"+q.Get("name")+":"+q.Get("value"))
	_ = json.NewEncoder(w).Encode(f.route(r, q))
}

func (f *fakeSAB) route(r *http.Request, q map[string][]string) any {
	get := func(k string) string { return first(q[k]) }
	switch get("mode") {
	case "queue":
		return f.queueMode(get("name"), get("value"))
	case "history":
		if get("name") == "delete" {
			f.history = drop(f.history, get("value"))
			return map[string]any{"status": true}
		}
		return map[string]any{"history": map[string]any{"slots": f.history}}
	case "addurl", "addfile":
		return f.add(r, get("nzbname"), get("cat"))
	case "switch":
		return map[string]any{"result": map[string]any{"position": get("value2")}}
	case "get_cats":
		return map[string]any{"categories": f.cats}
	case "pause", "resume":
		f.paused = get("mode") == "pause"
		return map[string]any{"status": true}
	case "config":
		f.limit = get("value")
		return map[string]any{"status": true}
	case "set_config":
		if f.locked {
			return map[string]any{"status": false, "error": "Configuration locked"}
		}
		f.cats = append(f.cats, strings.ToLower(get("name")))
		return map[string]any{"config": map[string]any{"categories": []any{map[string]any{"name": get("name"), "dir": get("dir")}}}}
	}
	return map[string]any{"status": false, "error": "not implemented"}
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

func (f *fakeSAB) queueMode(name, value string) any {
	switch name {
	case "delete":
		f.queue = drop(f.queue, value)
		return map[string]any{"status": true}
	case "pause", "resume":
		return map[string]any{"status": true}
	}
	for i, s := range f.queue {
		s["index"] = i
	}
	return map[string]any{"queue": map[string]any{"paused": f.paused, "kbpersec": "512.0", "speedlimit_abs": "102400",
		"diskspace1": "2.0", "slots": f.queue}}
}

func (f *fakeSAB) add(r *http.Request, name, cat string) any {
	if r.Method == http.MethodPost {
		if err := r.ParseMultipartForm(1 << 20); err != nil || r.MultipartForm.File["name"] == nil {
			return map[string]any{"status": false, "error": "no file"}
		}
	}
	f.next++
	id := "SABnzbd_nzo_" + strconv.Itoa(f.next)
	f.queue = append(f.queue, map[string]any{"nzo_id": id, "filename": name, "cat": cat, "status": "Queued",
		"percentage": "0", "mb": "100.0", "mbleft": "100.0", "timeleft": "0:00:00"})
	return map[string]any{"status": true, "nzo_ids": []string{id}}
}

func drop(list []map[string]any, id string) []map[string]any {
	return slices.DeleteFunc(list, func(m map[string]any) bool { return m["nzo_id"] == id })
}

func (f *fakeSAB) called(c string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.calls, c)
}

func newClient(t *testing.T, f *fakeSAB, extra plugin.Settings) *Client {
	t.Helper()
	s := plugin.Settings{"url": f.srv.URL + "/", "apiKey": f.key}
	for k, v := range extra {
		s[k] = v
	}
	p, err := New(plugin.Instance{Key: "sab", Name: "SAB", PluginID: ID, Settings: s, Env: conformance.Env(http.DefaultClient)})
	if err != nil {
		t.Fatal(err)
	}
	return p.(*Client)
}

func TestConformance(t *testing.T) {
	reg, ok := plugin.Default.Get(ID)
	if !ok {
		t.Fatal("sabnzbd not registered")
	}
	conformance.Manifest(t, reg.Manifest, plugin.KindDownloader)
	f := newFake(t)
	conformance.Downloader(t, newClient(t, f, nil), plugin.AddRequest{Payload: plugin.Payload{Kind: "nzb", URL: "https://ix.example/get/1"}, Category: "movies"})
	conformance.Downloader(t, newClient(t, f, nil), plugin.AddRequest{Payload: plugin.Payload{Kind: "nzb", Content: []byte("<nzb/>"), Name: "Show.S01E01"}, Category: "tv"})
}

func TestErrorMapping(t *testing.T) {
	conformance.ErrorMapping(t, func(url string) plugin.Tester {
		return newClient(t, &fakeSAB{srv: &httptest.Server{URL: url}, key: "k"}, nil)
	},
		conformance.ErrorCase{Name: "wrong key", Want: plugin.ClassAuth,
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"status":false,"error":"API Key Incorrect"}`))
			}},
		conformance.ErrorCase{Name: "garbage", Want: plugin.ClassParse,
			Handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }},
		conformance.ErrorCase{Name: "other error", Want: plugin.ClassServerError,
			Handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":false}`)) }},
		conformance.ErrorCase{Name: "teapot", Want: plugin.ClassServerError,
			Handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }},
	)
}

func TestSettingsAreValidated(t *testing.T) {
	for _, s := range []plugin.Settings{
		{"url": "ftp://x", "apiKey": "k"},
		{"url": "http://x"},
		{"url": "http://x", "apiKey": "k", "categories": map[string]any{"film": "x"}},
		{"url": "http://x", "apiKey": "k", "categories": map[string]any{"movie": " "}},
		{"url": "http://x", "apiKey": "k", "pathMap": []any{"nonsense"}},
	} {
		if _, err := New(plugin.Instance{Settings: s, Env: conformance.Env(http.DefaultClient)}); plugin.ClassOf(err) != plugin.ClassConfig {
			t.Errorf("%v: %v", s, err)
		}
	}
	c := newClient(t, newFake(t), plugin.Settings{"categories": map[string]any{"movie": "films"}})
	if c.CategoryFor(domain.KindMovie) != "films" || c.CategoryFor(domain.KindTV) != "tv" || c.Protocols()[0] != domain.ProtoUsenet {
		t.Fatal("categories or protocols")
	}
}

func TestListMapsQueueAndHistory(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f, plugin.Settings{"pathMap": []any{"/downloads=/mnt/dl"}})
	f.queue = []map[string]any{{"nzo_id": "a", "filename": "A", "cat": "movies", "status": "Downloading", "percentage": "50",
		"mb": "200", "mbleft": "100", "timeleft": "1:02:03"}}
	f.history = []map[string]any{
		{"nzo_id": "b", "name": "B", "category": "movies", "status": "Completed", "storage": "/downloads/B", "bytes": 10},
		{"nzo_id": "c", "name": "C", "category": "movies", "status": "Failed", "fail_message": "missing articles"},
		{"nzo_id": "d", "name": "D", "category": "movies", "status": "Extracting"},
		{"nzo_id": "e", "name": "E", "category": "other", "status": "Completed"}}
	snaps, err := c.List(context.Background(), []string{"movies"})
	if err != nil || len(snaps) != 4 {
		t.Fatalf("%v %v", snaps, err)
	}
	a, b, cc, d := snaps[0], snaps[1], snaps[2], snaps[3]
	if a.State != domain.StateDownloading || a.Progress != 0.5 || a.ETASeconds != 3723 || a.Downloaded != 100*mib {
		t.Errorf("queue slot %+v", a)
	}
	if b.State != domain.StateCompleted || b.ContentPath != "/mnt/dl/B" || len(b.Files) != 1 {
		t.Errorf("completed %+v", b)
	}
	if cc.State != domain.StateFailed || cc.Error != "missing articles" || d.State != domain.StateDownloading {
		t.Errorf("failed/working %+v %+v", cc, d)
	}
	if clock("x:1") != 0 {
		t.Error("bad clock")
	}
}

func TestPriorityMovesInTheQueue(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f, nil)
	ctx := context.Background()
	for _, n := range []string{"a", "b", "c"} {
		f.queue = append(f.queue, map[string]any{"nzo_id": n, "filename": n, "status": "Queued"})
	}
	if err := c.SetPriority(ctx, "b", plugin.PrioDown); err != nil || !f.called("switch::b") {
		t.Fatal(err)
	}
	if err := c.SetPriority(ctx, "b", "sideways"); plugin.ClassOf(err) != plugin.ClassConfig {
		t.Fatal(err)
	}
	if err := c.SetPriority(ctx, "gone", plugin.PrioTop); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[plugin.Priority]int{plugin.PrioTop: 0, plugin.PrioBottom: 2, plugin.PrioUp: 0, plugin.PrioDown: 2} {
		if got, _ := position(p, 1, 3); got != want && p != plugin.PrioUp {
			t.Errorf("%s: %d", p, got)
		}
	}
}

func TestPriorityLevels(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f, nil)
	for level, v := range map[plugin.Priority]string{plugin.PrioLow: "-1", plugin.PrioNormal: "0", plugin.PrioHigh: "1", plugin.PrioForce: "2"} {
		if err := c.SetPriorityLevel(t.Context(), "a", level); err != nil || !f.called("queue:priority:a") {
			t.Errorf("%s (%s): %v", level, v, err)
		}
	}
	if err := c.SetPriorityLevel(t.Context(), "a", plugin.PrioTop); plugin.ClassOf(err) != plugin.ClassConfig {
		t.Errorf("a move is not a level: %v", err)
	}
}

func TestEnsureCategoryCreatesMissingOnes(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f, plugin.Settings{"pathMap": []any{"/downloads=/mnt/dl"}})
	ctx := t.Context()
	if err := c.EnsureCategory(ctx, "Movies", ""); err != nil || f.called("set_config::") {
		t.Fatalf("existing category, any case: %v", err)
	}
	if err := c.EnsureCategory(ctx, "books", "/mnt/dl/books"); err != nil || !slices.Contains(f.cats, "books") {
		t.Fatalf("create: %v %v", err, f.cats)
	}
	f.locked = true
	err := c.EnsureCategory(ctx, "music", "")
	if plugin.ClassOf(err) != plugin.ClassConfig || !strings.Contains(err.Error(), `create the category "music" in SABnzbd`) {
		t.Fatalf("locked: %v", err)
	}
}

func TestClientWideControls(t *testing.T) {
	f := newFake(t)
	c := newClient(t, f, nil)
	ctx := context.Background()
	if err := c.PauseAll(ctx); err != nil || !f.paused {
		t.Fatal(err)
	}
	st, err := c.Stats(ctx)
	if err != nil || !st.Paused || st.DownSpeed != 512*1024 || st.FreeSpace != 2<<30 || st.Limits.DownKiB != 100 {
		t.Fatalf("%+v %v", st, err)
	}
	if err := c.ResumeAll(ctx); err != nil || f.paused {
		t.Fatal(err)
	}
	if err := c.SetSpeedLimits(ctx, plugin.SpeedLimits{DownKiB: 300}); err != nil || f.limit != "300K" {
		t.Fatal(err, f.limit)
	}
	if err := c.SetSpeedLimits(ctx, plugin.SpeedLimits{}); err != nil || f.limit != "100" {
		t.Fatal(err, f.limit)
	}
	if l, err := c.SpeedLimits(ctx); err != nil || l.DownKiB != 100 {
		t.Fatal(l, err)
	}
	if err := c.SetSpeedLimits(ctx, plugin.SpeedLimits{DownKiB: -1}); plugin.ClassOf(err) != plugin.ClassConfig {
		t.Fatal(err)
	}
	for _, err := range []error{c.SetSpeedLimits(ctx, plugin.SpeedLimits{UpKiB: 5}), c.SetAltSpeed(ctx, true), second(c.AltSpeed(ctx))} {
		if !errors.Is(err, plugin.ErrUnsupported) {
			t.Errorf("want unsupported: %v", err)
		}
	}
	if _, err := c.Add(ctx, plugin.AddRequest{}); plugin.ClassOf(err) != plugin.ClassParse {
		t.Fatal(err)
	}
	if !strings.HasPrefix(jobName(plugin.Payload{URL: "u"}), "corsarr-") {
		t.Fatal("job name")
	}
}

func second(_ bool, err error) error { return err }
