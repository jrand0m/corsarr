// Package scheduler runs recurring kernel and module work: interval jobs that never overlap themselves (ticks coalesce),
// run-now, exponential backoff on failure, persisted last run and result,
// live interval and enable changes, a global cap on heavy jobs, and
// `job.updated` events. Due times use the monotonic clock, so wall-clock
// jumps on boards without an RTC do not stampede or stall jobs.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Func is the work of one run. It must be idempotent: running it twice, or
// after a crash halfway, ends in the same state.
type Func func(ctx context.Context) error

// Job is a registration.
type Job struct {
	Name     string
	Interval time.Duration
	Heavy    bool // counts against the global heavy-job cap
	Run      Func
}

// StateStore persists job state (store.Jobs implements it).
type StateStore interface {
	Save(ctx context.Context, j domain.JobState) error
	List(ctx context.Context) ([]domain.JobState, error)
}

// Publisher is the event bus.
type Publisher interface {
	Publish(domain.Event) (domain.Event, error)
}

// Options configure a Scheduler; zero values take defaults.
type Options struct {
	HeavyCap    int           // default 2
	BaseBackoff time.Duration // default 30 s
	MaxBackoff  time.Duration // default 6 h
	Store       StateStore
	Events      Publisher
	OnError     func(job string, err error) // e.g. log
}

// Scheduler owns all job state; its loop goroutine is the only one that
// starts runs.
type Scheduler struct {
	o       Options
	mu      sync.Mutex
	jobs    map[string]*entry
	heavy   chan struct{}
	wake    chan struct{}
	runs    sync.WaitGroup
	cancel  context.CancelFunc
	ctx     context.Context
	stopped chan struct{}
}

type entry struct {
	job     Job
	state   domain.JobState
	due     time.Time // monotonic
	running bool
}

// ErrUnknownJob is returned for names that are not registered.
var ErrUnknownJob = errors.New("unknown job")

// New returns a stopped scheduler.
func New(o Options) *Scheduler {
	if o.HeavyCap <= 0 {
		o.HeavyCap = 2
	}
	if o.BaseBackoff <= 0 {
		o.BaseBackoff = 30 * time.Second
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = 6 * time.Hour
	}
	return &Scheduler{o: o, jobs: map[string]*entry{}, heavy: make(chan struct{}, o.HeavyCap), wake: make(chan struct{}, 1)}
}

// NextDelay is the pure backoff rule: the interval after success, else
// BaseBackoff·2^(failures-1) capped at MaxBackoff.
func NextDelay(interval time.Duration, failures int, base, maxDelay time.Duration) time.Duration {
	if failures <= 0 {
		return interval
	}
	d := base
	for i := 1; i < failures && d < maxDelay; i++ {
		d *= 2
	}
	return min(d, maxDelay)
}

// Register adds a job before or after Start. Persisted state (last run,
// failures, interval edits) is restored; a run interrupted by a crash
// (started, never finished) is due at once.
func (s *Scheduler) Register(ctx context.Context, j Job) error {
	if j.Name == "" || j.Run == nil || j.Interval <= 0 {
		return fmt.Errorf("job needs a name, a func and a positive interval")
	}
	e := &entry{job: j, state: domain.JobState{Name: j.Name, Enabled: true, Interval: j.Interval, Heavy: j.Heavy}}
	e.due = time.Now().Add(j.Interval)
	if p, ok := s.persisted(ctx, j.Name); ok {
		restore(e, p, time.Now())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.jobs[j.Name]; dup {
		return fmt.Errorf("job %s registered twice", j.Name)
	}
	s.jobs[j.Name] = e
	s.poke()
	return nil
}

func (s *Scheduler) persisted(ctx context.Context, name string) (domain.JobState, bool) {
	if s.o.Store == nil {
		return domain.JobState{}, false
	}
	list, err := s.o.Store.List(ctx)
	if err != nil {
		return domain.JobState{}, false
	}
	for _, p := range list {
		if p.Name == name {
			return p, true
		}
	}
	return domain.JobState{}, false
}

// restore applies persisted state; the wall-clock NextRun becomes a
// monotonic due time relative to now.
func restore(e *entry, p domain.JobState, now time.Time) {
	e.state.Enabled, e.state.Interval = p.Enabled, p.Interval
	e.state.LastStart, e.state.LastFinish, e.state.LastResult = p.LastStart, p.LastFinish, p.LastResult
	e.state.LastError, e.state.Failures, e.state.NextRun = p.LastError, p.Failures, p.NextRun
	switch {
	case interrupted(p):
		e.due = now
	case p.NextRun != nil:
		e.due = now.Add(max(p.NextRun.Sub(now.Round(0)), 0))
	}
}

func interrupted(p domain.JobState) bool {
	return p.LastStart != nil && (p.LastFinish == nil || p.LastFinish.Before(*p.LastStart))
}

func (s *Scheduler) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Start runs the loop until Stop.
func (s *Scheduler) Start(ctx context.Context) {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.stopped = make(chan struct{})
	go s.loop()
}

func (s *Scheduler) loop() {
	defer close(s.stopped)
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		wait := s.startDue(time.Now())
		timer.Reset(wait)
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
		case <-s.wake:
		}
	}
}

// startDue starts every due, enabled, idle job and returns the time until
// the next due job (an hour when none).
func (s *Scheduler) startDue(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := time.Hour
	for _, e := range s.jobs {
		if !e.state.Enabled || e.running {
			continue // a tick while running coalesces into the running run
		}
		if d := e.due.Sub(now); d > 0 {
			next = min(next, d)
			continue
		}
		s.start(e)
	}
	return next
}

// start launches one run; caller holds mu.
func (s *Scheduler) start(e *entry) {
	e.running = true
	s.runs.Add(1)
	go s.run(e)
}

func (s *Scheduler) run(e *entry) {
	defer s.runs.Done()
	if e.job.Heavy {
		select {
		case s.heavy <- struct{}{}:
			defer func() { <-s.heavy }()
		case <-s.ctx.Done():
			s.finish(e, time.Now(), s.ctx.Err())
			return
		}
	}
	started := time.Now()
	s.mu.Lock()
	e.state.Running, e.state.LastStart = true, wall(started)
	snap := e.state
	s.mu.Unlock()
	s.publish(snap, true)
	err := safeRun(s.ctx, e.job.Run)
	s.finish(e, time.Now(), err)
}

func safeRun(ctx context.Context, f Func) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("job panicked: %v", p)
		}
	}()
	return f(ctx)
}

func (s *Scheduler) finish(e *entry, now time.Time, err error) {
	s.mu.Lock()
	e.running, e.state.Running, e.state.LastFinish = false, false, wall(now)
	e.state.LastResult, e.state.LastError = domain.JobOK, ""
	if err != nil {
		e.state.LastResult, e.state.LastError = domain.JobFailed, err.Error()
		e.state.Failures++
	} else {
		e.state.Failures = 0
	}
	delay := NextDelay(e.state.Interval, e.state.Failures, s.o.BaseBackoff, s.o.MaxBackoff)
	e.due, e.state.NextRun = now.Add(delay), wall(now.Add(delay))
	snap := e.state
	s.mu.Unlock()
	if err != nil && s.o.OnError != nil {
		s.o.OnError(e.job.Name, err)
	}
	s.publish(snap, true)
	s.poke()
}

// wall strips the monotonic reading for persistence and display.
func wall(t time.Time) *time.Time { w := t.Round(0).UTC(); return &w }

func (s *Scheduler) publish(st domain.JobState, persist bool) {
	if persist && s.o.Store != nil {
		if err := s.o.Store.Save(context.Background(), st); err != nil && s.o.OnError != nil {
			s.o.OnError(st.Name, fmt.Errorf("save job state: %w", err))
		}
	}
	if s.o.Events != nil {
		_, _ = s.o.Events.Publish(domain.Event{Type: domain.EvJobUpdated, Actor: "system", Data: st})
	}
}

// RunNow makes a job due immediately; a running job coalesces the request.
func (s *Scheduler) RunNow(name string) error {
	return s.update(name, func(e *entry) { e.due = time.Now() })
}

// SetInterval changes a job's interval live; the next run is rescheduled
// from the last finish (or from now when it never ran).
func (s *Scheduler) SetInterval(name string, d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("interval must be positive")
	}
	return s.update(name, func(e *entry) {
		e.state.Interval = d
		if !e.running && e.state.Failures == 0 {
			e.due = time.Now().Add(d)
			e.state.NextRun = wall(e.due)
		}
	})
}

// SetEnabled pauses or resumes a job.
func (s *Scheduler) SetEnabled(name string, on bool) error {
	return s.update(name, func(e *entry) { e.state.Enabled = on })
}

func (s *Scheduler) update(name string, f func(*entry)) error {
	s.mu.Lock()
	e, ok := s.jobs[name]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrUnknownJob, name)
	}
	f(e)
	snap := e.state
	s.mu.Unlock()
	s.publish(snap, true)
	s.poke()
	return nil
}

// List returns every job's state sorted by name.
func (s *Scheduler) List() []domain.JobState {
	s.mu.Lock()
	out := make([]domain.JobState, 0, len(s.jobs))
	for _, e := range s.jobs {
		out = append(out, e.state)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Stop cancels running jobs and waits for them until ctx ends (graceful
// shutdown within 10 s).
func (s *Scheduler) Stop(ctx context.Context) error {
	if s.cancel == nil {
		return nil
	}
	s.cancel()
	<-s.stopped
	done := make(chan struct{})
	go func() { s.runs.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("jobs still running at shutdown: %w", ctx.Err())
	}
}
