package testkit

import (
	"net/http"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestKit(t *testing.T) {
	if Store(t) == nil {
		t.Fatal("store")
	}
	r := &Recorder{}
	_, _ = r.Publish(domain.Event{Type: domain.EvGrabSent})
	_, _ = r.Publish(domain.Event{Type: domain.EvItemAdded})
	if len(r.Of(domain.EvGrabSent)) != 1 || len(r.All()) != 2 {
		t.Fatal("recorder")
	}
	srv := Serve(t, func(m Mux) {
		m.Handle("GET /x", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	})
	if code, body := Do(t, srv, "GET", "/x", ""); code != 200 || body != "ok" {
		t.Fatal(code, body)
	}
	n := 0
	Eventually(t, time.Second, "counter", func() bool { n++; return n > 2 })
}
