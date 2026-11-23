package rules

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/testkit"
)

var ctx = context.Background()

type world struct {
	files   map[string]bool // "live" state a rule acts on
	removed []string
}

func (w *world) rule(mode Mode, cap int) Rule {
	return Rule{ID: "purge", Mode: mode, Cap: cap,
		Find: func(context.Context) ([]Candidate, error) {
			var out []Candidate
			for _, k := range []string{"a", "b", "c"} {
				if w.files[k] {
					out = append(out, Candidate{Key: k, Evidence: map[string]any{"age": "8d"}, Ops: []string{"delete " + k}})
				}
			}
			return out, nil
		},
		Recheck: func(_ context.Context, c Candidate) (bool, error) { return w.files[c.Key], nil },
		Act: func(_ context.Context, c Candidate) (string, error) {
			if c.Key == "boom" {
				return "", errors.New("disk on fire")
			}
			w.files[c.Key] = false
			w.removed = append(w.removed, c.Key)
			return c.Key, nil
		},
		Undo: func(_ context.Context, p string) error { w.files[p] = true; return nil },
	}
}

func newRunner(t *testing.T, rules ...Rule) (*Runner, *testkit.Recorder) {
	rec := &testkit.Recorder{}
	r := New(testkit.Store(t).Rules(), rec, nil)
	for _, rule := range rules {
		if err := r.Register(rule); err != nil {
			t.Fatal(err)
		}
	}
	return r, rec
}

func TestDryRunPersistsWouldActAndTouchesNothing(t *testing.T) {
	w := &world{files: map[string]bool{"a": true, "b": true}}
	r, rec := newRunner(t, w.rule(ModeDryRun, 0))
	counts, err := r.Run(ctx, "purge")
	if err != nil || counts[ClassWouldAct] != 2 || len(w.removed) != 0 {
		t.Fatalf("%v %v %v", counts, err, w.removed)
	}
	acts, _, _ := r.Actions(ctx, 0, 10)
	if len(acts) != 2 || acts[0].Status != ClassWouldAct || len(rec.Of(domain.EvRuleActed)) != 0 || len(rec.Of(domain.EvRuleRunCompleted)) != 1 {
		t.Fatalf("%+v", acts)
	}
}

func TestLiveRunHonoursCapAndUndo(t *testing.T) {
	w := &world{files: map[string]bool{"a": true, "b": true, "c": true}}
	r, rec := newRunner(t, w.rule(ModeLive, 2))
	counts, err := r.Run(ctx, "purge")
	if err != nil || counts[ClassActed] != 2 || counts[ClassDeferred] != 1 || len(w.removed) != 2 {
		t.Fatalf("%v %v %v", counts, err, w.removed)
	}
	if len(rec.Of(domain.EvRuleActed)) != 2 {
		t.Fatal("rule.acted per action")
	}
	acts, _, _ := r.Actions(ctx, 0, 10)
	var acted int64
	for _, a := range acts {
		if a.Status == "acted" {
			acted = a.ID
		}
	}
	a, err := r.Undo(ctx, acted)
	if err != nil || a.Status != "undone" || !w.files[a.Candidate] || len(rec.Of(domain.EvRuleUndone)) != 1 {
		t.Fatalf("undo %+v %v", a, err)
	}
	if _, err := r.Undo(ctx, acted); !errors.Is(err, ErrNoUndo) {
		t.Fatalf("undo twice: %v", err)
	}
	if _, err := r.Undo(ctx, 999); !errors.Is(err, ErrUnknownRule) {
		t.Fatalf("unknown action: %v", err)
	}
}

func TestApplyRechecksAndRecordsFailures(t *testing.T) {
	w := &world{files: map[string]bool{"a": true}}
	r, _ := newRunner(t, w.rule(ModeLive, 0))
	a, err := r.Apply(ctx, "purge", Candidate{Key: "gone"})
	if err != nil || a.Classification != ClassAmbig || a.Error != "state changed" {
		t.Fatalf("stale candidate: %+v %v", a, err)
	}
	w.files["boom"] = true
	a, err = r.Apply(ctx, "purge", Candidate{Key: "boom"})
	if err != nil || a.Status != "failed" || !strings.Contains(a.Error, "fire") {
		t.Fatalf("failed act: %+v %v", a, err)
	}
	a, err = r.Apply(ctx, "purge", Candidate{Key: "a"})
	if err != nil || a.Status != "acted" || !a.UndoAvailable {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := r.Apply(ctx, "nope", Candidate{}); !errors.Is(err, ErrUnknownRule) {
		t.Fatal(err)
	}
}

func TestRegisterAndOffMode(t *testing.T) {
	w := &world{files: map[string]bool{"a": true}}
	r, _ := newRunner(t, w.rule(ModeOff, 0))
	if err := r.Register(w.rule(ModeOff, 0)); err == nil {
		t.Fatal("duplicate id")
	}
	if err := r.Register(Rule{ID: "x"}); err == nil {
		t.Fatal("rule without action")
	}
	if counts, err := r.Run(ctx, "purge"); err != nil || len(counts) != 0 || len(w.removed) != 0 {
		t.Fatal("off mode does nothing")
	}
	if _, err := r.Run(ctx, "nope"); !errors.Is(err, ErrUnknownRule) {
		t.Fatal(err)
	}
	bad := w.rule(ModeLive, 0)
	bad.ID, bad.Find = "bad", func(context.Context) ([]Candidate, error) { return nil, errors.New("scan failed") }
	_ = r.Register(bad)
	if _, err := r.Run(ctx, "bad"); err == nil {
		t.Fatal("find error")
	}
}

func TestConfiguredModesOverrideTheDefault(t *testing.T) {
	w := &world{files: map[string]bool{"a": true}}
	r, _ := newRunner(t, w.rule(ModeDryRun, 0))
	if unknown := r.SetModes(map[string]Mode{"purge": ModeLive, "ghost": ModeOff}); len(unknown) != 1 || unknown[0] != "ghost" {
		t.Fatalf("unknown ids: %v", unknown)
	}
	if m, _ := r.Mode("purge"); m != ModeLive {
		t.Fatal(m)
	}
	if counts, err := r.Run(ctx, "purge"); err != nil || counts[ClassActed] != 1 || len(w.removed) != 1 {
		t.Fatalf("switched live: %v %v", counts, err)
	}
	r.SetModes(nil) // the key removed: back to the default
	if m, _ := r.Mode("purge"); m != ModeDryRun {
		t.Fatal(m)
	}
	if _, err := r.Mode("nope"); !errors.Is(err, ErrUnknownRule) {
		t.Fatal(err)
	}
}

func TestUndoWindowExpires(t *testing.T) {
	w := &world{files: map[string]bool{"a": true}}
	clock := time.Now()
	rec := &testkit.Recorder{}
	r := New(testkit.Store(t).Rules(), rec, func() time.Time { return clock })
	_ = r.Register(w.rule(ModeLive, 0))
	a, _ := r.Apply(ctx, "purge", Candidate{Key: "a"})
	clock = clock.Add(UndoWindow + time.Hour)
	if _, err := r.Undo(ctx, a.ID); !errors.Is(err, ErrNoUndo) {
		t.Fatalf("expired: %v", err)
	}
}

func TestHTTP(t *testing.T) {
	w := &world{files: map[string]bool{"a": true}}
	r, _ := newRunner(t, w.rule(ModeLive, 0))
	a, _ := r.Apply(ctx, "purge", Candidate{Key: "a"})
	srv := testkit.Serve(t, func(m testkit.Mux) { r.Mount(m) })
	if code, body := testkit.Do(t, srv, "GET", "/api/v1/rules/actions", ""); code != 200 || !strings.Contains(body, `"candidate":"a"`) {
		t.Fatal(code, body)
	}
	if code, _ := testkit.Do(t, srv, "GET", "/api/v1/rules/actions?limit=900", ""); code != 400 {
		t.Fatal(code)
	}
	if code, body := testkit.Do(t, srv, "POST", "/api/v1/rules/actions/"+itoa(a.ID)+"/undo", ""); code != 200 || !strings.Contains(body, "undone") {
		t.Fatal(code, body)
	}
	for path, want := range map[string]int{"/api/v1/rules/actions/" + itoa(a.ID) + "/undo": 409, "/api/v1/rules/actions/77/undo": 404, "/api/v1/rules/actions/x/undo": 400} {
		if code, _ := testkit.Do(t, srv, "POST", path, ""); code != want {
			t.Errorf("%s: %d", path, code)
		}
	}
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }
