// Package events is the in-process event bus:
// publishers never block, each subscriber has a bounded buffer that drops
// the oldest stream-only event first, events marked History are persisted
// without loss, and an SSE hub serves the stream at /api/v1/events.
package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// SchemaVersion is the envelope version this build publishes.
const SchemaVersion = 1

// HistoryWriter persists events marked H (store.History implements it).
type HistoryWriter interface {
	Append(ctx context.Context, ev domain.Event) error
}

// Bus fans events out to subscribers and to History.
type Bus struct {
	mu   sync.RWMutex
	subs map[*Sub]struct{}
	now  func() time.Time

	hist     HistoryWriter
	histMu   sync.Mutex
	histQ    []domain.Event // unbounded on purpose: History never drops
	histWake chan struct{}
	histDone chan struct{}
	histErr  func(error)
	closed   bool
}

// Option configures a Bus.
type Option func(*Bus)

// WithHistory persists Persisted() events through w; onErr sees write errors.
func WithHistory(w HistoryWriter, onErr func(error)) Option {
	return func(b *Bus) { b.hist, b.histErr = w, onErr }
}

// WithClock overrides time.Now (tests).
func WithClock(now func() time.Time) Option { return func(b *Bus) { b.now = now } }

// New starts a bus; Close drains History.
func New(opts ...Option) *Bus {
	b := &Bus{subs: map[*Sub]struct{}{}, now: time.Now, histWake: make(chan struct{}, 1), histDone: make(chan struct{})}
	for _, o := range opts {
		o(b)
	}
	go b.persist()
	return b
}

// ErrUnknownEvent rejects names outside the catalogue.
var ErrUnknownEvent = errors.New("event type not in the catalogue")

// Publish stamps and delivers an event. It never blocks.
func (b *Bus) Publish(ev domain.Event) (domain.Event, error) {
	if !ev.Type.Known() {
		return ev, fmt.Errorf("%w: %q", ErrUnknownEvent, ev.Type)
	}
	ev = b.stamp(ev)
	b.mu.RLock()
	defer b.mu.RUnlock()
	for s := range b.subs {
		s.push(ev)
	}
	if ev.Type.Persisted() && b.hist != nil && !b.closed {
		b.enqueueHistory(ev)
	}
	return ev, nil
}

func (b *Bus) stamp(ev domain.Event) domain.Event {
	if ev.SchemaVersion == 0 {
		ev.SchemaVersion = SchemaVersion
	}
	if ev.EventID == "" {
		ev.EventID = newID()
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = b.now().UTC()
	}
	if ev.Actor == "" {
		ev.Actor = "system"
	}
	return ev
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand never fails on supported platforms
	return hex.EncodeToString(b[:])
}

func (b *Bus) enqueueHistory(ev domain.Event) {
	b.histMu.Lock()
	b.histQ = append(b.histQ, ev)
	b.histMu.Unlock()
	select {
	case b.histWake <- struct{}{}:
	default:
	}
}

// persist is the only History writer of the bus; it drains the queue in order.
func (b *Bus) persist() {
	defer close(b.histDone)
	for range b.histWake {
		b.flushHistory()
	}
	b.flushHistory()
}

func (b *Bus) flushHistory() {
	for _, ev := range b.takeHistory() {
		if err := b.hist.Append(context.Background(), ev); err != nil && b.histErr != nil {
			b.histErr(err)
		}
	}
}

func (b *Bus) takeHistory() []domain.Event {
	b.histMu.Lock()
	defer b.histMu.Unlock()
	q := b.histQ
	b.histQ = nil
	return q
}

// Close stops History persistence after writing everything queued, and
// closes all subscriptions. Publishing after Close still reaches no one.
func (b *Bus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	for s := range b.subs {
		s.close()
		delete(b.subs, s)
	}
	b.mu.Unlock()
	close(b.histWake) // persist drains what is queued, then exits
	<-b.histDone
}

// Subscribe registers a subscriber with a bounded buffer of size n that only
// receives events accepted by filter (nil = all).
func (b *Bus) Subscribe(n int, filter func(domain.EventType) bool) *Sub {
	s := &Sub{bus: b, max: max(n, 1), filter: filter, wake: make(chan struct{}, 1)}
	b.mu.Lock()
	if b.closed {
		s.closed = true
	} else {
		b.subs[s] = struct{}{}
	}
	b.mu.Unlock()
	return s
}

// Subscribers is the number of live subscriptions.
func (b *Bus) Subscribers() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}

// Sub is one subscription.
type Sub struct {
	bus     *Bus
	mu      sync.Mutex
	buf     []domain.Event
	max     int
	filter  func(domain.EventType) bool
	wake    chan struct{}
	closed  bool
	dropped atomic.Int64
}

// push appends without blocking; when full, the oldest stream-only event is
// dropped first (progress, state), else the oldest event.
func (s *Sub) push(ev domain.Event) {
	if s.filter != nil && !s.filter(ev.Type) {
		return
	}
	s.mu.Lock()
	if len(s.buf) >= s.max {
		i := oldestDroppable(s.buf)
		s.buf = append(s.buf[:i], s.buf[i+1:]...)
		s.dropped.Add(1)
	}
	s.buf = append(s.buf, ev)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func oldestDroppable(buf []domain.Event) int {
	for i, e := range buf {
		if !e.Type.Persisted() {
			return i
		}
	}
	return 0
}

// ErrClosed is returned by Next after Close.
var ErrClosed = errors.New("subscription closed")

// Next blocks until an event arrives, ctx ends or the subscription closes.
func (s *Sub) Next(ctx context.Context) (domain.Event, error) {
	for {
		s.mu.Lock()
		if len(s.buf) > 0 {
			ev := s.buf[0]
			s.buf = s.buf[1:]
			s.mu.Unlock()
			return ev, nil
		}
		closed := s.closed
		s.mu.Unlock()
		if closed {
			return domain.Event{}, ErrClosed
		}
		select {
		case <-s.wake:
		case <-ctx.Done():
			return domain.Event{}, ctx.Err()
		}
	}
}

// Dropped counts events lost to a full buffer.
func (s *Sub) Dropped() int64 { return s.dropped.Load() }

// Close unsubscribes; safe to call twice.
func (s *Sub) Close() {
	s.bus.mu.Lock()
	delete(s.bus.subs, s)
	s.bus.mu.Unlock()
	s.close()
}

func (s *Sub) close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
