package queue

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/testkit"
)

func TestQueueHTTPAndUserActions(t *testing.T) {
	e := newEnv(t)
	d := e.grab(t, "aaa")
	id := strconv.FormatInt(d.ID, 10)
	srv := testkit.Serve(t, func(m testkit.Mux) { e.svc.Mount(m) })
	runs := e.jobs.runs
	steps := []struct {
		method, path, body string
		code               int
		has                string
	}{
		{"GET", "/api/v1/queue", "", 200, `"downloadId":"aaa"`},
		{"GET", "/api/v1/queue?limit=500", "", 400, ""},
		{"POST", "/api/v1/queue/" + id + "/pause", "", 200, `"state":"queued"`},
		{"POST", "/api/v1/queue/" + id + "/resume", "", 200, ""},
		{"POST", "/api/v1/queue/" + id + "/priority", `{"priority":"top"}`, 200, ""},
		{"POST", "/api/v1/queue/" + id + "/hold", "", 200, `"held":true`},
		{"POST", "/api/v1/queue/" + id + "/release", "", 200, `"held":false`},
		{"POST", "/api/v1/queue/" + id + "/explode", "", 404, ""},
		{"POST", "/api/v1/queue/x/pause", "", 400, ""},
		{"POST", "/api/v1/queue/99/pause", "", 404, ""},
		{"POST", "/api/v1/queue/" + id + "/remove", `{"deleteFiles":true,"blocklist":true,"research":true}`, 200, `"state":"removed"`},
		{"POST", "/api/v1/queue/" + id + "/remove", `{"bad":1}`, 400, ""},
	}
	for _, s := range steps {
		code, body := testkit.Do(t, srv, s.method, s.path, s.body)
		if code != s.code || !strings.Contains(body, s.has) {
			t.Errorf("%s %s: %d %s", s.method, s.path, code, body)
		}
	}
	if strings.Join(e.dl.ops, ",") != "pause,resume,prio:top" || e.jobs.runs <= runs {
		t.Fatalf("ops %v runs %d", e.dl.ops, e.jobs.runs)
	}
	rm := e.rec.Of(domain.EvDownloadRemoved)
	if len(rm) != 1 {
		t.Fatal("removed")
	}
	data := rm[0].Data.(domain.DownloadEventData)
	if !data.Blocklist || !data.Research || !data.FilesDeleted || data.Cause != "user" || rm[0].Actor != "user" || !e.dl.removed["aaa"] {
		t.Fatalf("%+v", data)
	}
	// removing again is a no-op
	if err := e.svc.Remove(ctx, d.ID, RemoveOptions{}); err != nil || len(e.rec.Of(domain.EvDownloadRemoved)) != 1 {
		t.Fatal(err)
	}
}

func TestClientErrorsMapToStatuses(t *testing.T) {
	e := newEnv(t)
	d := e.grab(t, "aaa")
	srv := testkit.Serve(t, func(m testkit.Mux) { e.svc.Mount(m) })
	path := "/api/v1/queue/" + strconv.FormatInt(d.ID, 10)
	e.dl.opErr = plugin.Errorf(plugin.ClassConfig, "enable queueing")
	if code, _ := testkit.Do(t, srv, "POST", path+"/priority", `{"priority":"top"}`); code != 400 {
		t.Fatal(code)
	}
	e.dl.opErr = errors.New("gone")
	if code, _ := testkit.Do(t, srv, "POST", path+"/pause", ""); code != 502 {
		t.Fatal(code)
	}
	if code, _ := testkit.Do(t, srv, "POST", path+"/remove", `{}`); code != 502 {
		t.Fatal(code)
	}
	e.svc.d.Plugins = &plugins{ix: e.ix}
	if code, _ := testkit.Do(t, srv, "POST", path+"/pause", ""); code != 503 {
		t.Fatal(code)
	}
}
