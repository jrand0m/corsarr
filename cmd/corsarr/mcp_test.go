package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const stdioScript = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"smoke"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}

{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_health","arguments":{}}}
`

func checkStdioAnswers(t *testing.T, out string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("one answer per request, none for the notification: %q", out)
	}
	for i, want := range []string{`"serverInfo"`, `"find_media"`, `"integrations"`} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("answer %d lacks %s: %s", i+1, want, lines[i])
		}
	}
	if strings.Contains(lines[1], `"request_movie"`) {
		t.Error("read scope lists no write tools")
	}
}

// With the data dir free, `mcp stdio` runs corsarr in-process.
func TestMCPStdioInProcess(t *testing.T) {
	withStdin(t, stdioScript)
	code, out, errs := cliRun(t, nil, "mcp", "stdio", "--data.dir="+t.TempDir(), "--mcp.enabled=true")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	checkStdioAnswers(t, out)
}

func TestMCPStdioNeedsEnabled(t *testing.T) {
	if code, _, errs := cliRun(t, nil, "mcp", "stdio", "--data.dir="+t.TempDir()); code != 1 || !strings.Contains(errs, "mcp.enabled") {
		t.Fatalf("%d %s", code, errs)
	}
	if code, _, _ := cliRun(t, nil, "mcp", "stdio", "--log.level=loud"); code != 1 {
		t.Fatal("invalid config")
	}
}

// While a server holds the data dir, `mcp stdio` relays to its endpoint.
func TestMCPStdioRelaysToRunningServer(t *testing.T) {
	dir := t.TempDir()
	args := []string{"--data.dir=" + dir, "--server.address=127.0.0.1:0", "--server.basePath=/c", "--mcp.enabled=true"}
	c := parse(args, []string{"HOME=" + t.TempDir()}, &bytes.Buffer{}, &bytes.Buffer{})
	ctx, cancel := context.WithCancel(context.Background())
	addrc, done := make(chan string, 1), make(chan int, 1)
	go func() { done <- c.serve(ctx, func(a string) { addrc <- a }) }()
	defer func() { cancel(); <-done }()
	var addr string
	select {
	case addr = <-addrc:
	case <-time.After(10 * time.Second):
		t.Fatal("server not ready")
	}
	withStdin(t, stdioScript)
	code, out, errs := cliRun(t, nil, "mcp", "stdio", "--data.dir="+dir, "--server.address="+addr, "--server.basePath=/c", "--mcp.enabled=true")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	checkStdioAnswers(t, out)
}

func TestRelayReadsEventStreamsAndReportsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n")
	}))
	defer srv.Close()
	var out bytes.Buffer
	r := &relay{url: srv.URL, client: srv.Client(), out: &out}
	if err := r.send(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)); err != nil || out.String() != "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n" {
		t.Fatalf("%v %q", err, out.String())
	}
	if err := r.send(context.Background(), []byte("  ")); err != nil {
		t.Fatal("blank lines are skipped")
	}
	srv.Close()
	if err := r.send(context.Background(), []byte(`{}`)); err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Fatal(err)
	}
	if mcpURL(":8090", "/c") != "http://127.0.0.1:8090/c/mcp" {
		t.Fatal(mcpURL(":8090", "/c"))
	}
}

// lockedBuffer is a bytes.Buffer safe for the relay's two writers.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// The relay forwards the session's notification stream to stdout,
// reconnecting after a server error and stopping when the session is gone.
func TestRelayForwardsNotifications(t *testing.T) {
	old := streamRetry
	streamRetry = time.Millisecond
	t.Cleanup(func() { streamRetry = old })
	var gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Mcp-Session-Id", "s1")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
			return
		}
		if r.Header.Get("Mcp-Session-Id") != "s1" || r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch gets.Add(1) {
		case 1:
			w.WriteHeader(http.StatusBadGateway)
		case 2:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, ": keep-alive\n\nevent: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/resources/updated\",\"params\":{\"uri\":\"corsarr://queue\"}}\n\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	out := &lockedBuffer{}
	r := &relay{url: srv.URL, key: "k", client: srv.Client(), streamer: srv.Client(), out: out}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.send(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)); err != nil {
		t.Fatal(err)
	}
	_ = r.send(ctx, []byte(`{"jsonrpc":"2.0","id":2,"method":"ping"}`)) // one stream per session
	r.wait()
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || !strings.Contains(out.String(), `"uri":"corsarr://queue"`) || gets.Load() != 3 {
		t.Fatalf("%d gets: %q", gets.Load(), out.String())
	}
}

// A cancelled relay stops listening.
func TestRelayStreamStopsWithTheContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Mcp-Session-Id", "s1")
			_, _ = io.WriteString(w, `{}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	r := &relay{url: srv.URL, client: srv.Client(), streamer: srv.Client(), out: &lockedBuffer{}}
	ctx, cancel := context.WithCancel(context.Background())
	if err := r.send(ctx, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	cancel()
	r.wait()
}
