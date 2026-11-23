package rootfolders

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/testkit"
)

var ctx = context.Background()

func TestAssess(t *testing.T) {
	for _, c := range []struct {
		p     Probe
		want  domain.RootFolderProblem
		class string
	}{
		{Probe{}, domain.RootMissing, health.ClassPath},
		{Probe{Dir: true, Free: 1 << 40}, domain.RootReadOnly, health.ClassPath},
		{Probe{Dir: true, Writable: true, Free: 10}, domain.RootFull, health.ClassDiskFull},
		{Probe{Dir: true, Writable: true, Free: Reserve}, domain.RootOK, ""},
	} {
		got, o := Assess(c.p, Reserve)
		if got != c.want || o.Class != c.class || o.OK != (c.want == domain.RootOK) {
			t.Errorf("%+v: %q %+v", c.p, got, o)
		}
	}
}

func TestCheckStoresReportsAndServes(t *testing.T) {
	st := testkit.Store(t)
	reg := health.New(nil, nil, nil)
	roots := []Configured{{Path: "/ok", Kind: domain.KindMovie}, {Path: "/gone", Kind: domain.KindMovie}, {Path: "/full", Kind: domain.KindTV}}
	probes := map[string]Probe{"/ok": {Dir: true, Writable: true, Free: 5e9}, "/full": {Dir: true, Writable: true, Free: 1}}
	c := New(Deps{Roots: func() []Configured { return roots }, Repo: st.RootFolders(), Health: reg,
		Probe: func(p string) Probe { return probes[p] }, Now: func() time.Time { return time.Unix(1e9, 0) },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err := c.Check(ctx); err != nil {
		t.Fatal(err)
	}
	list, _ := c.List(ctx)
	if len(list) != 3 || list[0].Path != "/full" || list[0].Problem != domain.RootFull || list[1].Problem != domain.RootMissing ||
		list[2].Problem != domain.RootOK || list[2].FreeSpace != 5e9 || !list[2].Writable || !list[2].Managed || list[2].LastChecked == nil {
		t.Fatalf("%+v", list)
	}
	if h, _ := reg.Get(Ref("/gone")); h.Status != domain.HealthDegraded || h.LastErrorClass != health.ClassPath {
		t.Fatalf("missing folder health: %+v", h)
	}
	if h, _ := reg.Get(Ref("/full")); h.LastErrorClass != health.ClassDiskFull {
		t.Fatalf("full folder health: %+v", h)
	}
	srv := testkit.Serve(t, func(m testkit.Mux) { c.Mount(m) })
	if code, body := testkit.Do(t, srv, "GET", "/api/v1/rootfolders", ""); code != 200 || !strings.Contains(body, `"problem":"missing"`) {
		t.Fatal(code, body)
	}
	// a folder dropped from the config leaves the store and its health is disabled
	roots = roots[:1]
	_ = c.Check(ctx)
	if list, _ := c.List(ctx); len(list) != 1 || list[0].Path != "/ok" {
		t.Fatalf("%+v", list)
	}
	if h, _ := reg.Get(Ref("/gone")); h.Status != domain.HealthDisabled {
		t.Fatalf("dropped folder: %+v", h)
	}
}

func TestProbePath(t *testing.T) {
	dir := t.TempDir()
	if p := ProbePath(dir); !p.Dir || !p.Writable || p.Free <= 0 {
		t.Fatalf("%+v", p)
	}
	if p := ProbePath(dir + "/nope"); p.Dir || FreeSpace(dir+"/nope") != 0 {
		t.Fatalf("%+v", p)
	}
	c := New(Deps{})
	if c.d.Reserve != Reserve || c.d.Probe == nil {
		t.Fatal("defaults")
	}
}
