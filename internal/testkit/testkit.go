// Package testkit holds small helpers shared by the module tests: a temp
// store, an event recorder and an HTTP harness for mounted routes. It is
// imported only from _test.go files.
package testkit

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// Store opens a fresh database in a temp dir, closed at cleanup.
func Store(t testing.TB) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// Recorder is a Publisher that keeps every event.
type Recorder struct {
	mu     sync.Mutex
	events []domain.Event
	n      int
}

// Publish records the event and stamps an id.
func (r *Recorder) Publish(ev domain.Event) (domain.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	ev.EventID = "ev" + string(rune('a'+r.n%26)) + time.Now().Format("150405.000000000")
	r.events = append(r.events, ev)
	return ev, nil
}

// Of returns the recorded events of one type.
func (r *Recorder) Of(t domain.EventType) []domain.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Event
	for _, e := range r.events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

// All returns every recorded event.
func (r *Recorder) All() []domain.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.Event(nil), r.events...)
}

// Mux is an api.Mounter over a plain ServeMux (no auth).
type Mux struct{ *http.ServeMux }

// Handle registers a route.
func (m Mux) Handle(pattern string, h http.HandlerFunc) { m.ServeMux.Handle(pattern, h) }

// Serve mounts routes on a test server.
func Serve(t testing.TB, mounts ...func(Mux)) *httptest.Server {
	t.Helper()
	m := Mux{http.NewServeMux()}
	for _, mount := range mounts {
		mount(m)
	}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return srv
}

// Do sends a request and returns the status and body.
func Do(t testing.TB, srv *httptest.Server, method, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// Eventually polls cond until it holds or the timeout passes.
func Eventually(t testing.TB, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
