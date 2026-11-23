package events

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Hub serves the SSE stream of events marked S. Clients do not resume:
// Last-Event-ID is ignored and clients refetch state after reconnecting.
type Hub struct {
	bus       *Bus
	heartbeat time.Duration
	buffer    int
	clients   atomic.Int64
}

// NewHub builds the SSE handler; heartbeat keeps proxies from closing idle
// streams (15 s is a good default).
func NewHub(bus *Bus, heartbeat time.Duration) *Hub {
	return &Hub{bus: bus, heartbeat: heartbeat, buffer: 256}
}

// Clients is the number of connected streams ("UI connected" signal).
func (h *Hub) Clients() int { return int(h.clients.Load()) }

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "retry: 3000\n\n")
	flusher.Flush()

	h.clients.Add(1)
	defer h.clients.Add(-1)
	sub := h.bus.Subscribe(h.buffer, domain.EventType.Streamed)
	defer sub.Close()
	h.stream(w, flusher, r, sub)
}

func (h *Hub) stream(w io.Writer, f http.Flusher, r *http.Request, sub *Sub) {
	events := pump(r.Context(), sub)
	tick := time.NewTicker(h.heartbeat)
	defer tick.Stop()
	for {
		select {
		case ev, ok := <-events:
			if !ok || writeEvent(w, ev) != nil {
				return
			}
		case <-tick.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
		}
		f.Flush()
	}
}

// pump turns a subscription into a channel that closes when ctx ends (net/http
// cancels it when ServeHTTP returns) or the subscription closes.
func pump(ctx context.Context, sub *Sub) <-chan domain.Event {
	out := make(chan domain.Event)
	go func() {
		defer close(out)
		for {
			ev, err := sub.Next(ctx)
			if err != nil {
				return
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func writeEvent(w io.Writer, ev domain.Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
	return err
}
