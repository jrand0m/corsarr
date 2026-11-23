package events

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

type memHistory struct {
	mu   sync.Mutex
	evs  []domain.Event
	fail bool
}

func (m *memHistory) Append(_ context.Context, ev domain.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("disk full")
	}
	m.evs = append(m.evs, ev)
	return nil
}

func (m *memHistory) types() []domain.EventType {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.EventType
	for _, e := range m.evs {
		out = append(out, e.Type)
	}
	return out
}

func TestPublishStampsAndRejectsUnknown(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	b := New(WithClock(func() time.Time { return at }))
	defer b.Close()
	ev, err := b.Publish(domain.Event{Type: domain.EvGrabSent})
	if err != nil || ev.EventID == "" || ev.SchemaVersion != 1 || !ev.OccurredAt.Equal(at) || ev.Actor != "system" {
		t.Fatalf("%+v %v", ev, err)
	}
	if _, err := b.Publish(domain.Event{Type: "grab.happened"}); !errors.Is(err, ErrUnknownEvent) {
		t.Fatalf("unknown: %v", err)
	}
	ev2, _ := b.Publish(domain.Event{Type: domain.EvGrabSent, EventID: "x", Actor: "user"})
	if ev2.EventID != "x" || ev2.Actor != "user" {
		t.Fatal("caller values must be kept")
	}
}

func TestPersistedEventsReachHistoryInOrderWithoutLoss(t *testing.T) {
	h := &memHistory{}
	b := New(WithHistory(h, nil))
	for i := 0; i < 500; i++ {
		_, _ = b.Publish(domain.Event{Type: domain.EvGrabSent})
		_, _ = b.Publish(domain.Event{Type: domain.EvDownloadProgress}) // not persisted
	}
	b.Close()
	got := h.types()
	if len(got) != 500 {
		t.Fatalf("history got %d events", len(got))
	}
	for _, ty := range got {
		if ty != domain.EvGrabSent {
			t.Fatalf("non-persisted event stored: %s", ty)
		}
	}
	b.Close() // idempotent
	if _, err := b.Publish(domain.Event{Type: domain.EvGrabSent}); err != nil {
		t.Fatal("publish after close must not fail or panic")
	}
}

func TestHistoryErrorsAreReported(t *testing.T) {
	h := &memHistory{fail: true}
	var mu sync.Mutex
	var errs []error
	b := New(WithHistory(h, func(err error) { mu.Lock(); errs = append(errs, err); mu.Unlock() }))
	_, _ = b.Publish(domain.Event{Type: domain.EvImportFailed})
	b.Close()
	if len(errs) != 1 {
		t.Fatalf("errors: %v", errs)
	}
}

func TestSlowSubscriberDropsOldestStreamOnlyAndNeverBlocks(t *testing.T) {
	b := New()
	defer b.Close()
	s := b.Subscribe(3, nil)
	_, _ = b.Publish(domain.Event{Type: domain.EvGrabSent, EventID: "g1"})
	_, _ = b.Publish(domain.Event{Type: domain.EvDownloadProgress, EventID: "p1"})
	_, _ = b.Publish(domain.Event{Type: domain.EvDownloadProgress, EventID: "p2"})
	done := make(chan struct{})
	go func() {
		_, _ = b.Publish(domain.Event{Type: domain.EvGrabSent, EventID: "g2"}) // must not block
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publisher blocked")
	}
	var ids []string
	for i := 0; i < 3; i++ {
		ev, _ := s.Next(context.Background())
		ids = append(ids, ev.EventID)
	}
	if strings.Join(ids, ",") != "g1,p2,g2" || s.Dropped() != 1 {
		t.Fatalf("got %v dropped %d", ids, s.Dropped())
	}
	// all persisted: the oldest goes
	for _, id := range []string{"a", "b", "c", "d"} {
		_, _ = b.Publish(domain.Event{Type: domain.EvGrabSent, EventID: id})
	}
	ev, _ := s.Next(context.Background())
	if ev.EventID != "b" {
		t.Fatalf("got %s", ev.EventID)
	}
}

func TestFilterCloseAndContext(t *testing.T) {
	b := New()
	s := b.Subscribe(10, domain.EventType.Persisted)
	_, _ = b.Publish(domain.Event{Type: domain.EvDownloadProgress})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := s.Next(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("filtered event delivered: %v", err)
	}
	if b.Subscribers() != 1 {
		t.Fatal("subscribers")
	}
	s.Close()
	s.Close()
	if _, err := s.Next(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	s2 := b.Subscribe(0, nil)
	b.Close()
	if _, err := s2.Next(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatal("bus close must close subscriptions")
	}
	if s3 := b.Subscribe(1, nil); !s3.closed {
		t.Fatal("subscribe after close")
	}
}

func TestSSEStreamsOnlyStreamedEventsWithHeartbeat(t *testing.T) {
	b := New()
	defer b.Close()
	hub := NewHub(b, 20*time.Millisecond)
	srv := httptest.NewServer(hub)
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Last-Event-ID", "ignored")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal(resp.Header)
	}
	rd := bufio.NewReader(resp.Body)
	waitFor(t, func() bool { return hub.Clients() == 1 })
	_, _ = b.Publish(domain.Event{Type: domain.EvJobUpdated, Data: map[string]any{"name": "rss"}})
	var sawEvent, sawPing bool
	deadline := time.Now().Add(2 * time.Second)
	for (!sawEvent || !sawPing) && time.Now().Before(deadline) {
		line, err := rd.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		sawEvent = sawEvent || strings.HasPrefix(line, "event: job.updated")
		sawPing = sawPing || strings.HasPrefix(line, ": ping")
	}
	if !sawEvent || !sawPing {
		t.Fatalf("event=%v ping=%v", sawEvent, sawPing)
	}
	resp.Body.Close()
	waitFor(t, func() bool { return hub.Clients() == 0 && b.Subscribers() == 0 })
}

type noFlush struct{ http.ResponseWriter }

func TestSSENeedsFlusher(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHub(New(), time.Second).ServeHTTP(noFlush{rec}, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatal(rec.Code)
	}
}

func TestWriteEventMarshalError(t *testing.T) {
	if err := writeEvent(&strings.Builder{}, domain.Event{Data: make(chan int)}); err == nil {
		t.Fatal("want marshal error")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met")
}
