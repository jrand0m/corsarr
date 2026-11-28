package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/store"
)

// mcpStdio serves MCP on stdin/stdout for a local agent. With the data dir
// free it runs corsarr in this process; while a server holds the data dir it
// relays every message to that server's /mcp endpoint with the stored API
// key, so nothing new listens on the network either way.
func (c cli) mcpStdio(ctx context.Context) int {
	cfg, ok := c.load()
	if !ok {
		return 1
	}
	if !cfg.MCP.Enabled {
		fmt.Fprintln(c.stderr, "mcp.enabled is false; enable it to serve assistants")
		return 1
	}
	k, err := startKernel(ctx, cfg, newLogger(c, cfg.Log.Level), c.moduleStore())
	if errors.Is(err, store.ErrLocked) {
		return c.relayMCP(ctx, cfg)
	}
	if err != nil {
		fmt.Fprintln(c.stderr, "corsarr:", err)
		return 1
	}
	defer k.close()
	return c.exit(k.app.MCP.ServeStdio(ctx, stdin, c.stdout))
}

func (c cli) exit(err error) int {
	if err != nil {
		fmt.Fprintln(c.stderr, "corsarr mcp:", err)
		return 1
	}
	return 0
}

// relayMCP forwards newline-delimited messages to the running server.
func (c cli) relayMCP(ctx context.Context, cfg *config.Config) int {
	key, err := store.ReadAPIKey(ctx, cfg.Data.Dir)
	if err != nil {
		return c.exit(err)
	}
	r := &relay{url: mcpURL(cfg.Server.Address, cfg.Server.BasePath), key: key, out: c.stdout,
		client: &http.Client{Timeout: 5 * time.Minute}, streamer: &http.Client{}}
	ctx, cancel := context.WithCancel(ctx)
	defer r.wait()
	defer cancel()
	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() && ctx.Err() == nil {
		if err := r.send(ctx, sc.Bytes()); err != nil {
			return c.exit(err)
		}
	}
	return c.exit(sc.Err())
}

// mcpURL is the server's endpoint, on loopback when it listens everywhere.
func mcpURL(addr, basePath string) string {
	return strings.TrimSuffix(healthURL(addr, basePath), "/api/v1/health") + "/mcp"
}

type relay struct {
	url, key, session string
	client            *http.Client
	streamer          *http.Client // no timeout: the notification stream stays open
	out               io.Writer

	mu        sync.Mutex // keeps answers and notifications whole on out
	listening bool
	wg        sync.WaitGroup
}

func (r *relay) send(ctx context.Context, msg []byte) error {
	if len(bytes.TrimSpace(msg)) == 0 {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if r.session != "" {
		req.Header.Set("Mcp-Session-Id", r.session)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach corsarr at %s: %w", r.url, err)
	}
	defer resp.Body.Close()
	if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
		r.session = id
		r.listen(ctx)
	}
	return r.copy(resp)
}

// line writes one message as a line of its own.
func (r *relay) line(msg []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.out.Write(append(bytes.TrimSpace(msg), '\n'))
	return err
}

// listen opens the session's notification stream once, so resource change
// notifications reach the local client too.
func (r *relay) listen(ctx context.Context) {
	if r.listening || r.streamer == nil {
		return
	}
	r.listening = true
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.notifications(ctx, r.session)
	}()
}

func (r *relay) wait() { r.wg.Wait() }

// Notification stream reconnect delays.
var (
	streamRetry    = time.Second
	streamRetryMax = 30 * time.Second
)

// notifications relays the stream until ctx ends or the server says the
// session or stream is gone, reconnecting with backoff when it drops.
func (r *relay) notifications(ctx context.Context, session string) {
	delay := streamRetry
	for {
		connected, stop := r.streamOnce(ctx, session)
		if stop || ctx.Err() != nil {
			return
		}
		if connected {
			delay = streamRetry
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, streamRetryMax)
	}
}

// streamOnce reads one connection of the stream; stop is true when the
// server will not serve it (no session, no stream, one already open).
func (r *relay) streamOnce(ctx context.Context, session string) (connected, stop bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return false, true
	}
	req.Header.Set("Authorization", "Bearer "+r.key)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Mcp-Session-Id", session)
	resp, err := r.streamer.Do(req)
	if err != nil {
		return false, false
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK:
		r.events(resp.Body)
		return true, false
	case resp.StatusCode >= 500:
		return false, false
	}
	return false, true
}

// events writes each server-sent event's data as a line.
func (r *relay) events(body io.Reader) {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	var data []byte
	for sc.Scan() {
		line := sc.Bytes()
		if d, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			data = append(data, bytes.TrimSpace(d)...)
			continue
		}
		if len(line) == 0 && len(data) > 0 {
			if r.line(data) != nil {
				return
			}
			data = nil
		}
	}
}

// copy writes the answer as one line; 202 (a notification) has none and an
// event stream carries it in its data lines.
func (r *relay) copy(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode == http.StatusAccepted || len(body) == 0 {
		return err
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		body = sseData(body)
	}
	return r.line(body)
}

func sseData(stream []byte) []byte {
	var out []byte
	for _, line := range bytes.Split(stream, []byte("\n")) {
		if data, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			out = append(out, bytes.TrimSpace(data)...)
		}
	}
	return out
}
