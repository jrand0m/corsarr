package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

type memStore struct {
	mu sync.Mutex
	m  map[string]domain.JobState
}

func newMem() *memStore { return &memStore{m: map[string]domain.JobState{}} }

func (m *memStore) Save(_ context.Context, j domain.JobState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[j.Name] = j
	return nil
}

func (m *memStore) List(context.Context) ([]domain.JobState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.JobState
	for _, j := range m.m {
		out = append(out, j)
	}
	return out, nil
}

func (m *memStore) get(n string) domain.JobState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.m[n]
}

type evCount struct{ n atomic.Int32 }

func (e *evCount) Publish(ev domain.Event) (domain.Event, error) {
	if ev.Type == domain.EvJobUpdated {
		e.n.Add(1)
	}
	return ev, nil
}

func start(t *testing.T, o Options) *Scheduler {
	t.Helper()
	s := New(o)
	s.Start(context.Background())
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	return s
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met")
}

func TestNextDelay(t *testing.T) {
	cases := []struct {
		f    int
		want time.Duration
	}{{0, time.Hour}, {1, 30 * time.Second}, {2, time.Minute}, {4, 4 * time.Minute}, {30, 6 * time.Hour}}
	for _, c := range cases {
		if got := NextDelay(time.Hour, c.f, 30*time.Second, 6*time.Hour); got != c.want {
			t.Errorf("f=%d: %v", c.f, got)
		}
	}
}

func TestIntervalJobRunsPersistsAndPublishes(t *testing.T) {
	st, ev := newMem(), &evCount{}
	s := start(t, Options{Store: st, Events: ev})
	var n atomic.Int32
	if err := s.Register(context.Background(), Job{Name: "tick", Interval: 20 * time.Millisecond, Run: func(context.Context) error { n.Add(1); return nil }}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return n.Load() >= 3 })
	j := st.get("tick")
	if j.LastResult != domain.JobOK || j.LastFinish == nil || j.NextRun == nil || ev.n.Load() < 6 {
		t.Fatalf("state %+v events %d", j, ev.n.Load())
	}
	if l := s.List(); len(l) != 1 || l[0].Name != "tick" {
		t.Fatal(l)
	}
}

func TestTicksCoalesceWhileRunning(t *testing.T) {
	s := start(t, Options{})
	var running, maxRunning, runs atomic.Int32
	release := make(chan struct{})
	_ = s.Register(context.Background(), Job{Name: "slow", Interval: 5 * time.Millisecond, Run: func(context.Context) error {
		r := running.Add(1)
		if r > maxRunning.Load() {
			maxRunning.Store(r)
		}
		runs.Add(1)
		<-release
		running.Add(-1)
		return nil
	}})
	eventually(t, func() bool { return runs.Load() == 1 })
	_ = s.RunNow("slow")
	time.Sleep(50 * time.Millisecond)
	if runs.Load() != 1 || maxRunning.Load() != 1 {
		t.Fatalf("overlap: runs %d max %d", runs.Load(), maxRunning.Load())
	}
	close(release)
}

func TestRunNowStartsWithinASecond(t *testing.T) {
	s := start(t, Options{})
	ran := make(chan struct{}, 1)
	_ = s.Register(context.Background(), Job{Name: "daily", Interval: 24 * time.Hour, Run: func(context.Context) error { ran <- struct{}{}; return nil }})
	t0 := time.Now()
	if err := s.RunNow("daily"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("run now did not start")
	}
	if time.Since(t0) > time.Second {
		t.Fatal("slow")
	}
	if err := s.RunNow("nope"); !errors.Is(err, ErrUnknownJob) {
		t.Fatal(err)
	}
}

func TestFailureBacksOffAndOthersUnaffected(t *testing.T) {
	st := newMem()
	var errs atomic.Int32
	s := start(t, Options{Store: st, BaseBackoff: time.Hour, OnError: func(string, error) { errs.Add(1) }})
	var good atomic.Int32
	_ = s.Register(context.Background(), Job{Name: "bad", Interval: 10 * time.Millisecond, Run: func(context.Context) error { return errors.New("indexer down") }})
	_ = s.Register(context.Background(), Job{Name: "good", Interval: 10 * time.Millisecond, Run: func(context.Context) error { good.Add(1); return nil }})
	eventually(t, func() bool { return good.Load() >= 3 && st.get("bad").Failures == 1 })
	time.Sleep(30 * time.Millisecond)
	b := st.get("bad")
	if b.Failures != 1 || b.LastResult != domain.JobFailed || b.LastError != "indexer down" || b.NextRun.Sub(*b.LastFinish) < 59*time.Minute {
		t.Fatalf("backoff state: %+v", b)
	}
	if errs.Load() != 1 {
		t.Fatal(errs.Load())
	}
}

func TestPanickingJobIsAFailure(t *testing.T) {
	st := newMem()
	s := start(t, Options{Store: st, BaseBackoff: time.Hour})
	_ = s.Register(context.Background(), Job{Name: "p", Interval: time.Millisecond, Run: func(context.Context) error { panic("boom") }})
	eventually(t, func() bool { return st.get("p").LastResult == domain.JobFailed })
}

func TestLiveIntervalChangeAndEnable(t *testing.T) {
	s := start(t, Options{})
	var n atomic.Int32
	_ = s.Register(context.Background(), Job{Name: "j", Interval: time.Hour, Run: func(context.Context) error { n.Add(1); return nil }})
	time.Sleep(20 * time.Millisecond)
	if n.Load() != 0 {
		t.Fatal("ran early")
	}
	if err := s.SetInterval("j", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return n.Load() >= 2 })
	_ = s.SetEnabled("j", false)
	time.Sleep(20 * time.Millisecond)
	before := n.Load()
	time.Sleep(50 * time.Millisecond)
	if n.Load() > before+1 {
		t.Fatalf("disabled job kept running: %d → %d", before, n.Load())
	}
	if s.SetInterval("j", 0) == nil || s.SetInterval("x", time.Second) == nil {
		t.Fatal("bad interval/unknown accepted")
	}
}

func TestHeavyCap(t *testing.T) {
	s := start(t, Options{HeavyCap: 1})
	var cur, peak, done atomic.Int32
	run := func(context.Context) error {
		c := cur.Add(1)
		if c > peak.Load() {
			peak.Store(c)
		}
		time.Sleep(20 * time.Millisecond)
		cur.Add(-1)
		done.Add(1)
		return nil
	}
	for _, n := range []string{"a", "b", "c"} {
		_ = s.Register(context.Background(), Job{Name: n, Interval: time.Hour, Heavy: true, Run: run})
		_ = s.RunNow(n)
	}
	eventually(t, func() bool { return done.Load() == 3 })
	if peak.Load() != 1 {
		t.Fatalf("heavy cap exceeded: %d", peak.Load())
	}
}

func TestRestoreFromStore(t *testing.T) {
	st := newMem()
	past := time.Now().Add(-time.Minute).UTC()
	future := time.Now().Add(time.Hour).UTC()
	_ = st.Save(context.Background(), domain.JobState{Name: "crashed", Enabled: true, Interval: time.Hour, LastStart: &past})
	_ = st.Save(context.Background(), domain.JobState{Name: "later", Enabled: true, Interval: 2 * time.Hour, NextRun: &future, Failures: 2, LastFinish: &past, LastStart: &past})
	s := start(t, Options{Store: st})
	ran := make(chan string, 2)
	f := func(name string) Func { return func(context.Context) error { ran <- name; return nil } }
	_ = s.Register(context.Background(), Job{Name: "crashed", Interval: time.Hour, Run: f("crashed")})
	_ = s.Register(context.Background(), Job{Name: "later", Interval: time.Hour, Run: f("later")})
	select {
	case n := <-ran:
		if n != "crashed" {
			t.Fatalf("wrong job: %s", n)
		}
	case <-time.After(time.Second):
		t.Fatal("interrupted job did not rerun after restart")
	}
	for _, j := range s.List() {
		if j.Name == "later" && (j.Interval != 2*time.Hour || j.Failures != 2) {
			t.Fatalf("not restored: %+v", j)
		}
	}
}

func TestRegisterValidationAndStop(t *testing.T) {
	s := New(Options{})
	if s.Stop(context.Background()) != nil {
		t.Fatal("stop before start")
	}
	if s.Register(context.Background(), Job{Name: "x"}) == nil {
		t.Fatal("invalid job accepted")
	}
	j := Job{Name: "x", Interval: time.Second, Run: func(context.Context) error { return nil }}
	_ = s.Register(context.Background(), j)
	if s.Register(context.Background(), j) == nil {
		t.Fatal("duplicate accepted")
	}
	s.Start(context.Background())
	block := make(chan struct{})
	_ = s.Register(context.Background(), Job{Name: "stuck", Interval: time.Millisecond, Run: func(context.Context) error { <-block; return nil }})
	time.Sleep(20 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.Stop(ctx); err == nil {
		t.Fatal("stop should report a job ignoring cancellation")
	}
	close(block)
}

func TestHeavyJobWaitingAtShutdownFinishesCancelled(t *testing.T) {
	st := newMem()
	s := New(Options{HeavyCap: 1, Store: st})
	s.Start(context.Background())
	hold := make(chan struct{})
	_ = s.Register(context.Background(), Job{Name: "a", Interval: time.Hour, Heavy: true, Run: func(ctx context.Context) error { <-ctx.Done(); <-hold; return nil }})
	_ = s.Register(context.Background(), Job{Name: "b", Interval: time.Hour, Heavy: true, Run: func(context.Context) error { return nil }})
	_ = s.RunNow("a")
	time.Sleep(20 * time.Millisecond)
	_ = s.RunNow("b")
	time.Sleep(20 * time.Millisecond)
	go func() { time.Sleep(30 * time.Millisecond); close(hold) }()
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st.get("b").LastResult != domain.JobFailed {
		t.Fatalf("b: %+v", st.get("b"))
	}
}
