package queue

import (
	"context"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/testkit"
)

// capDL is a downloader with every optional capability.
type capDL struct {
	*fakeDL
	limits  plugin.SpeedLimits
	alt     bool
	paused  bool
	statErr error
}

func (c *capDL) PauseAll(context.Context) error  { c.paused = true; return c.op("pause-all") }
func (c *capDL) ResumeAll(context.Context) error { c.paused = false; return c.op("resume-all") }
func (c *capDL) SpeedLimits(context.Context) (plugin.SpeedLimits, error) {
	return c.limits, nil
}
func (c *capDL) SetSpeedLimits(_ context.Context, l plugin.SpeedLimits) error {
	c.limits = l
	return c.op("limits")
}
func (c *capDL) AltSpeed(context.Context) (bool, error)       { return c.alt, c.opErr }
func (c *capDL) SetAltSpeed(_ context.Context, on bool) error { c.alt = on; return c.op("alt") }
func (c *capDL) Recheck(context.Context, string) error        { return c.op("recheck") }
func (c *capDL) SetPriorityLevel(_ context.Context, _ string, l plugin.Priority) error {
	return c.op("level:" + string(l))
}
func (c *capDL) Reannounce(context.Context, string) error { return c.op("reannounce") }
func (c *capDL) SetCategory(_ context.Context, _, cat string) error {
	return c.op("category:" + cat)
}
func (c *capDL) Stats(context.Context) (plugin.ClientStatus, error) {
	return plugin.ClientStatus{DownSpeed: 10, FreeSpace: 99, Paused: c.paused, Limits: c.limits, AltSpeed: c.alt}, c.statErr
}
func (c *capDL) Trackers(context.Context, string) ([]plugin.Tracker, error) {
	return []plugin.Tracker{{URL: "https://t.example", Status: "working"}}, nil
}

// controlEnv has a capable client "qb" and a plain one "plain".
func controlEnv(t *testing.T) (*env, *capDL, *httptest.Server) {
	e := newEnv(t)
	c := &capDL{fakeDL: e.dl}
	e.svc.d.Plugins = &plugins{ix: e.ix, dls: []Downloader{{Key: "qb", Impl: c}, {Key: "plain", Impl: newDL()}}}
	srv := testkit.Serve(t, func(m testkit.Mux) { e.svc.Mount(m) })
	return e, c, srv
}

type step struct {
	method, path, body string
	code               int
	has                string
}

func run(t *testing.T, srv *httptest.Server, steps []step) {
	t.Helper()
	for _, s := range steps {
		code, body := testkit.Do(t, srv, s.method, s.path, s.body)
		if code != s.code || !strings.Contains(body, s.has) {
			t.Errorf("%s %s: %d %s", s.method, s.path, code, body)
		}
	}
}

func TestPerDownloadControl(t *testing.T) {
	e, c, srv := controlEnv(t)
	d := e.grab(t, "aaa")
	p := "/api/v1/queue/" + strconv.FormatInt(d.ID, 10)
	e.dl.files["aaa"] = []plugin.FileEntry{{Path: "a.mkv", Size: 1}}
	run(t, srv, []step{
		{"POST", p + "/recheck", "", 200, `"downloadId":"aaa"`},
		{"POST", p + "/reannounce", "", 200, ""},
		{"POST", p + "/category", `{"category":"hold"}`, 200, ""},
		{"POST", p + "/category", `{}`, 400, "category is required"},
		{"POST", p + "/retry-import", "", 409, "not_importable"},
		{"GET", p, "", 200, `"trackers":[{"url":"https://t.example"`},
		{"GET", "/api/v1/queue/99", "", 404, "not_found"},
		{"GET", "/api/v1/queue/x", "", 400, ""},
	})
	if strings.Join(c.ops, ",") != "recheck,reannounce,category:hold" {
		t.Fatalf("ops %v", c.ops)
	}
	_ = e.svc.Transition(ctx, d.ID, domain.StateCompleted, domain.ActorDownloader, "")
	before := len(e.rec.Of(domain.EvDownloadCompleted))
	run(t, srv, []step{{"POST", p + "/retry-import", "", 200, `"state":"completed"`}})
	if len(e.rec.Of(domain.EvDownloadCompleted)) != before+1 {
		t.Fatal("retry import must hand the download to the import")
	}
	_ = e.svc.Hold(ctx, d.ID, true)
	run(t, srv, []step{{"POST", p + "/retry-import", "", 409, "held"}})
	det, err := e.svc.Detail(ctx, d.ID)
	if err != nil || len(det.History) != 1 || det.History[0].To != domain.StateCompleted || len(det.Files) != 1 {
		t.Fatalf("detail %+v %v", det, err)
	}
}

func TestPriorityMovesAndLevels(t *testing.T) {
	e, c, srv := controlEnv(t)
	d := e.grab(t, "aaa")
	p := "/api/v1/queue/" + strconv.FormatInt(d.ID, 10) + "/priority"
	run(t, srv, []step{
		{"POST", p, `{"priority":"top"}`, 200, ""},
		{"POST", p, `{"priority":"force"}`, 200, ""},
		{"POST", p, `{"priority":"sideways"}`, 400, "unknown priority"},
	})
	if strings.Join(c.ops, ",") != "prio:top,level:force" {
		t.Fatalf("ops %v", c.ops)
	}
	e.svc.d.Plugins = &plugins{ix: e.ix, dls: []Downloader{{Key: "qb", Impl: e.dl}}} // no levels
	run(t, srv, []step{{"POST", p, `{"priority":"high"}`, 501, "unsupported"}})
}

// failImport walks a finished download through a failed import.
func failImport(t *testing.T, e *env, id int64) {
	t.Helper()
	for _, step := range []struct {
		to domain.DownloadState
		by domain.Actor
	}{{domain.StateCompleted, domain.ActorDownloader}, {domain.StateImportPending, domain.ActorImport},
		{domain.StateImporting, domain.ActorImport}, {domain.StateFailed, domain.ActorImport}} {
		if err := e.svc.Transition(ctx, id, step.to, step.by, ""); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRetryImportAfterAFailedImport(t *testing.T) {
	e, _, srv := controlEnv(t)
	d := e.grab(t, "aaa")
	d.ContentPath = "/dl/Example"
	if err := e.st.Queue().Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	failImport(t, e, d.ID)
	kept := false
	e.svc.d.PayloadExists = func(path string) bool { return kept && path == "/dl/Example" }
	p := "/api/v1/queue/" + strconv.FormatInt(d.ID, 10) + "/retry-import"
	run(t, srv, []step{{"POST", p, "", 409, "not_importable"}}) // the payload is gone
	kept = true
	before := len(e.rec.Of(domain.EvDownloadCompleted))
	run(t, srv, []step{{"POST", p, "", 200, `"state":"import-pending"`}})
	if len(e.rec.Of(domain.EvDownloadCompleted)) != before+1 {
		t.Fatal("retry import must hand the download to the import")
	}
	if got := states(e.rec); got[len(got)-1] != "failed>import-pending" {
		t.Fatalf("transitions %v", got)
	}

	never := e.grab(t, "bbb") // failed before it finished: nothing to import
	if err := e.svc.Transition(ctx, never.ID, domain.StateFailed, domain.ActorUser, ""); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RetryImport(ctx, never.ID); !errors.Is(err, ErrNotImportable) {
		t.Fatal(err)
	}
	e.svc.d.PayloadExists = nil // the real file system: the path does not exist
	if e.svc.payloadKept(d) {
		t.Fatal("a missing path counts as kept")
	}
}

func TestMissingCapabilityIsUnsupported(t *testing.T) {
	e, _, srv := controlEnv(t)
	d := e.grab(t, "aaa")
	p := "/api/v1/queue/" + strconv.FormatInt(d.ID, 10)
	e.svc.d.Plugins = &plugins{ix: e.ix, dls: []Downloader{{Key: "qb", Impl: e.dl}}} // the plain fake: no optional writes
	run(t, srv, []step{
		{"POST", p + "/recheck", "", 501, "unsupported"},
		{"POST", p + "/reannounce", "", 501, "unsupported"},
		{"POST", p + "/category", `{"category":"x"}`, 501, "unsupported"},
		{"POST", "/api/v1/downloaders/qb/pause-all", "", 501, "unsupported"},
		{"POST", "/api/v1/downloaders/qb/speed-limits", `{"downKiB":1}`, 501, "unsupported"},
		{"POST", "/api/v1/downloaders/qb/alt-speed", "", 501, "unsupported"},
		{"GET", p, "", 200, `"capabilities":["files"]`},
	})
	if err := e.svc.Recheck(ctx, d.ID); !errors.Is(err, plugin.ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestQueueFilters(t *testing.T) {
	e, _, srv := controlEnv(t)
	a, _ := e.grab(t, "aaa"), e.grab(t, "bbb")
	_ = e.svc.Transition(ctx, a.ID, domain.StateCompleted, domain.ActorDownloader, "")
	run(t, srv, []step{
		{"GET", "/api/v1/queue?state=completed&kind=movie&downloader=qb", "", 200, `"downloadId":"aaa"`},
		{"GET", "/api/v1/queue?state=queued,stalled", "", 200, `"downloadId":"bbb"`},
		{"GET", "/api/v1/queue?downloader=plain", "", 200, `"items":null`},
		{"GET", "/api/v1/queue?state=bogus", "", 400, "unknown state"},
		{"GET", "/api/v1/queue?kind=film", "", 400, ""},
	})
	if items, _, _ := e.svc.ListFiltered(ctx, Filter{States: []domain.DownloadState{domain.StateQueued}}, 0, 10); len(items) != 1 {
		t.Fatalf("%d", len(items))
	}
}

func TestPerClientControl(t *testing.T) {
	e, c, srv := controlEnv(t)
	run(t, srv, []step{
		{"GET", "/api/v1/downloaders", "", 200, `"key":"plain"`},
		{"GET", "/api/v1/downloaders/qb", "", 200, `"capabilities":["files","priority-levels","trackers","pause-all","speed-limits","recheck","reannounce","set-category","stats"]`},
		{"POST", "/api/v1/downloaders/qb/pause-all", "", 200, `"paused":true`},
		{"POST", "/api/v1/downloaders/qb/resume-all", "", 200, `"paused":false`},
		{"POST", "/api/v1/downloaders/qb/speed-limits", `{"downKiB":500,"upKiB":50}`, 200, `"downKiB":500`},
		{"POST", "/api/v1/downloaders/qb/speed-limits", `{"downKiB":-1}`, 400, ""},
		{"POST", "/api/v1/downloaders/qb/alt-speed", "", 200, `"altSpeed":true`},
		{"POST", "/api/v1/downloaders/qb/alt-speed", `{"on":true}`, 200, `"altSpeed":true`},
		{"POST", "/api/v1/downloaders/qb/test", "", 200, ""},
		{"POST", "/api/v1/downloaders/qb/explode", "", 404, ""},
		{"GET", "/api/v1/downloaders/nope", "", 404, "not_found"},
		{"POST", "/api/v1/downloaders/nope/test", "", 404, "not_found"},
	})
	c.statErr = errors.New("down")
	if v, err := e.svc.Client(ctx, "qb"); err != nil || v.StatsError != "down" || v.Stats != nil {
		t.Fatalf("%+v %v", v, err)
	}
	c.opErr = errors.New("boom")
	if _, err := e.svc.SetAltSpeed(ctx, "qb", nil); err == nil {
		t.Fatal("reading the mode failed")
	}
	tested := domain.ObjectKey("")
	e.svc.d.TestClient = func(_ context.Context, k domain.ObjectKey) error { tested = k; return errors.New("unreachable") }
	run(t, srv, []step{{"POST", "/api/v1/downloaders/qb/test", "", 502, "unreachable"}})
	if tested != "qb" {
		t.Fatal("host test not used")
	}
}

func TestClientWritesAreShadowedWhileAnyKindIsInShadow(t *testing.T) {
	e, c, srv := controlEnv(t)
	e.svc.d.Modes = func(k domain.MediaKind) domain.KindMode {
		return map[domain.MediaKind]domain.KindMode{domain.KindMusic: domain.ModeShadow}[k]
	}
	run(t, srv, []step{
		{"POST", "/api/v1/downloaders/qb/pause-all", "", 409, "shadowed"},
		{"POST", "/api/v1/downloaders/qb/speed-limits", `{"downKiB":5}`, 409, "shadowed"},
		{"POST", "/api/v1/downloaders/qb/test", "", 200, ""}, // a read is never shadowed
		{"GET", "/api/v1/downloaders", "", 200, `"stats"`},
	})
	sh := e.rec.Of(domain.EvClientShadowed)
	if len(c.ops) != 0 || len(sh) != 2 || sh[1].Data.(domain.ClientActionData).Args["downKiB"] != int64(5) || sh[0].Kind != domain.KindMusic {
		t.Fatalf("ops %v shadowed %+v", c.ops, sh)
	}
}
