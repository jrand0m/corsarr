package importer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestRebase(t *testing.T) {
	if p, ok := Rebase("/a/m/x.mkv", "/a/m", "/b/m"); !ok || p != "/b/m/x.mkv" {
		t.Fatal(p)
	}
	for _, outside := range []string{"/a/other/x.mkv", "/a/m2/x.mkv", "/a"} {
		if _, ok := Rebase(outside, "/a/m", "/b/m"); ok {
			t.Errorf("%s rebased", outside)
		}
	}
}

func (e *env) imported(t *testing.T) string {
	t.Helper()
	e.payload(t, map[string]string{"movie.mkv": "the movie", "sub/extra.srt": "s"})
	e.completed(t, 7)
	return e.q.outcomes[0].Items[0].NewPaths[0]
}

func TestMoveItemRenamesAndRecordsTheMove(t *testing.T) {
	e := newEnv(t)
	file := e.imported(t)
	from, to := e.mv.m.Path, filepath.Join(filepath.Dir(e.lib), "other", filepath.Base(e.mv.m.Path))
	plan, _, _ := e.svc.PlanMove(ctx, e.mv.m.Ref(), from, to)
	if len(plan.Files) != 1 || plan.Files[0].From != file || exists(to) {
		t.Fatalf("preview changes nothing: %+v", plan)
	}
	plan, err := e.svc.MoveItem(ctx, e.mv.m.Ref(), from, to)
	moved := filepath.Join(to, filepath.Base(file))
	if err != nil || exists(from) || !exists(moved) || plan.Files[0].To != moved {
		t.Fatalf("%v %+v", err, plan)
	}
	if f, _, _ := e.st.Library().File(ctx, e.mv.m.FileID); f.Path != moved {
		t.Fatalf("record: %s", f.Path)
	}
	ev := e.rec.Of(domain.EvFileRenamed)
	if len(ev) != 1 || ev[0].Data.(domain.FileEventData).OldPath != file || ev[0].Data.(domain.FileEventData).Reason != "moved" {
		t.Fatalf("%+v", ev)
	}
	if _, err := e.svc.MoveItem(ctx, e.mv.m.Ref(), from, to); err != nil || !exists(moved) {
		t.Fatal("moving again is a no-op", err)
	}
	if _, err := e.svc.MoveItem(ctx, e.mv.m.Ref(), to, from); err != nil || !exists(file) {
		t.Fatal("moving back undoes it", err)
	}
}

func TestMoveItemCopiesAcrossFilesystemsAndRefusesToMerge(t *testing.T) {
	e := newEnv(t)
	file := e.imported(t)
	rename = func(string, string) error { return errors.New("cross-device link") }
	defer func() { rename = os.Rename }()
	from := e.mv.m.Path
	to := filepath.Join(t.TempDir(), "elsewhere", filepath.Base(from))
	if _, err := e.svc.MoveItem(ctx, e.mv.m.Ref(), from, to); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(to, filepath.Base(file)))
	if err != nil || string(body) != "the movie" || exists(from) {
		t.Fatalf("copied, verified, source removed: %v %q", err, body)
	}
	if err := os.MkdirAll(from, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.MoveItem(ctx, e.mv.m.Ref(), to, from); !errors.Is(err, ErrMoveConflict) {
		t.Fatalf("existing destination: %v", err)
	}
	e.svc.d.FreeSpace = func(string) int64 { return 0 }
	if err := os.Remove(from); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.MoveItem(ctx, e.mv.m.Ref(), to, from); err == nil || exists(from) || !exists(to) {
		t.Fatalf("a failed copy leaves the source and no partial destination: %v", err)
	}
	e.svc.d.Modes = func(domain.MediaKind) domain.KindMode { return domain.ModeShadow }
	if _, err := e.svc.MoveItem(ctx, e.mv.m.Ref(), to, from); err == nil || !strings.Contains(err.Error(), "shadow") {
		t.Fatalf("shadow: %v", err)
	}
}
