package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
