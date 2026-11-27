package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
)

// keyed serves the endpoint with a scope chosen by the X-Test-Key header.
func (h *harness) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope := map[string]api.KeyScope{"full": api.ScopeFull, "read": api.ScopeRead}[r.Header.Get("X-Test-Key")]
		h.s.ServeHTTP(w, r, scope)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func send(t *testing.T, srv *httptest.Server, method, key, session, body string, hdr ...string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+"/mcp", strings.NewReader(body))
	req.Header.Set("X-Test-Key", key)
	if session != "" {
		req.Header.Set(headerSession, session)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

const initMsg = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"t"}}}`

func TestHTTPGatesAndSessions(t *testing.T) {
	h := newHarness(t)
	srv := h.server(t)
	ping := `{"jsonrpc":"2.0","id":2,"method":"ping"}`
	for name, c := range map[string]struct {
		key, body string
		hdr       []string
		status    int
	}{
		"no key":          {"", initMsg, nil, 401},
		"foreign origin":  {"full", initMsg, []string{"Origin", "http://evil.example"}, 403},
		"bad version":     {"full", initMsg, []string{headerProtocol, "1999-01-01"}, 400},
		"no session":      {"full", ping, nil, 400},
		"unknown session": {"full", ping, []string{headerSession, "nope"}, 404},
		"not json":        {"full", "{", nil, 400},
	} {
		if resp := send(t, srv, "POST", c.key, "", c.body, c.hdr...); resp.StatusCode != c.status {
			t.Errorf("%s: %d", name, resp.StatusCode)
		}
	}
	if resp := send(t, srv, "POST", "", "", initMsg); resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("401 names the scheme")
	}
	resp := send(t, srv, "POST", "full", "", initMsg, "Origin", srv.URL)
	id := resp.Header.Get(headerSession)
	if resp.StatusCode != 200 || id == "" {
		t.Fatalf("initialize: %d", resp.StatusCode)
	}
	if resp := send(t, srv, "POST", "read", id, ping); resp.StatusCode != 404 {
		t.Fatal("a session belongs to its key")
	}
	if resp := send(t, srv, "POST", "full", id, ping, headerProtocol, "2025-11-25"); resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	if resp := send(t, srv, "POST", "full", id, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); resp.StatusCode != 202 {
		t.Fatal("notifications are accepted", resp.StatusCode)
	}
	if resp := send(t, srv, "PUT", "full", id, ""); resp.StatusCode != 405 {
		t.Fatal(resp.StatusCode)
	}
	if resp := send(t, srv, "DELETE", "full", id, ""); resp.StatusCode != 204 {
		t.Fatal(resp.StatusCode)
	}
	if resp := send(t, srv, "DELETE", "full", id, ""); resp.StatusCode != 404 {
		t.Fatal("ended")
	}
	if resp := send(t, srv, "POST", "full", "", strings.Repeat("x", maxBody+1)); resp.StatusCode != 413 {
		t.Fatal(resp.StatusCode)
	}
	h.setMCP(config.MCP{Enabled: true, Scope: config.MCPScopeRead, AllowedOrigins: []string{"http://inspector.local"}})
	if resp := send(t, srv, "POST", "full", "", initMsg, "Origin", "http://inspector.local"); resp.StatusCode != 200 {
		t.Fatal("configured origins pass")
	}
	h.setMCP(config.MCP{})
	if resp := send(t, srv, "POST", "full", "", initMsg); resp.StatusCode != 404 {
		t.Fatal("disabled answers 404")
	}
}

func TestMountRegistersOnScopedMounters(t *testing.T) {
	h := newHarness(t)
	m := &recordMounter{}
	h.s.Mount(m)
	if m.pattern != "/mcp" {
		t.Fatal(m.pattern)
	}
	h.s.Mount(plainMounter{}) // not scoped: nothing to do
}

type recordMounter struct{ pattern string }

func (m *recordMounter) Handle(string, http.HandlerFunc) {}
func (m *recordMounter) HandleScoped(p string, _ func(http.ResponseWriter, *http.Request, api.KeyScope)) {
	m.pattern = p
}

type plainMounter struct{}

func (plainMounter) Handle(string, http.HandlerFunc) {}

// Subscribed resources stream change notifications over GET.
func TestStreamDeliversResourceUpdates(t *testing.T) {
	h := newHarness(t)
	old := KeepAlive
	KeepAlive = 20 * time.Millisecond
	t.Cleanup(func() { KeepAlive = old })
	srv := h.server(t)
	id := send(t, srv, "POST", "full", "", initMsg).Header.Get(headerSession)
	send(t, srv, "POST", "full", id, `{"jsonrpc":"2.0","id":2,"method":"resources/subscribe","params":{"uri":"corsarr://queue"}}`)
	if resp := send(t, srv, "GET", "full", "", ""); resp.StatusCode != 400 {
		t.Fatal("a stream needs a session")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.s.Run(ctx)
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/mcp", nil)
	req.Header.Set("X-Test-Key", "full")
	req.Header.Set(headerSession, id)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if second := send(t, srv, "GET", "full", id, ""); second.StatusCode != 409 {
		t.Fatal("one stream per session", second.StatusCode)
	}
	rd := bufio.NewReader(resp.Body)
	waitLine(t, rd, ": keep-alive")
	time.Sleep(50 * time.Millisecond) // Run has subscribed
	_, _ = h.bus.Publish(domain.Event{Type: domain.EvGrabSent, Actor: "test"})
	if line := waitLine(t, rd, "data:"); !strings.Contains(line, `"uri":"corsarr://queue"`) {
		t.Fatal(line)
	}
}

func waitLine(t *testing.T, rd *bufio.Reader, prefix string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err := rd.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("no %q line", prefix)
	return ""
}

func TestStdio(t *testing.T) {
	h := newHarness(t)
	in := strings.Join([]string{initMsg, "", `{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_health"}}`}, "\n")
	var out strings.Builder
	if err := h.s.ServeStdio(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], "integrations") {
		t.Fatalf("%q", out.String())
	}
	if err := h.s.ServeStdio(context.Background(), strings.NewReader(initMsg+"\n"), failWriter{}); err == nil {
		t.Fatal("write errors end the session")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestStdioForwardsNotifications(t *testing.T) {
	h := newHarness(t)
	pr, pw := io.Pipe()
	var out safeBuffer
	done := make(chan error, 1)
	go func() { done <- h.s.ServeStdio(context.Background(), pr, &out) }()
	_, _ = io.WriteString(pw, `{"jsonrpc":"2.0","id":1,"method":"resources/subscribe","params":{"uri":"corsarr://health"}}`+"\n")
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(out.String(), `"id":1`) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	h.s.publish(context.Background(), domain.Event{Type: domain.EvHealthChanged})
	for !strings.Contains(out.String(), "notifications/resources/updated") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	pw.Close()
	if err := <-done; err != nil || !strings.Contains(out.String(), "corsarr://health") {
		t.Fatal(err, out.String())
	}
}

type safeBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestResources(t *testing.T) {
	h := newHarness(t)
	var list struct {
		Resources  []resourceInfo `json:"resources"`
		NextCursor string         `json:"nextCursor"`
	}
	_ = json.Unmarshal(h.rpc(t, "resources/list", nil).Result, &list)
	if len(list.Resources) != 4 || list.Resources[0].URI != URIQueue || list.Resources[2].URI != "corsarr://item/movie/1" || list.NextCursor != "1:0" {
		t.Fatalf("%+v", list)
	}
	cursor := list.NextCursor
	list.Resources, list.NextCursor = nil, ""
	_ = json.Unmarshal(h.rpc(t, "resources/list", map[string]string{"cursor": cursor}).Result, &list)
	if len(list.Resources) != 1 || list.Resources[0].URI != "corsarr://item/tv/1" || list.NextCursor != "2:0" {
		t.Fatalf("%+v", list)
	}
	list.NextCursor = ""
	_ = json.Unmarshal(h.rpc(t, "resources/list", map[string]string{"cursor": "2:0"}).Result, &list)
	if len(list.Resources) != 1 || list.NextCursor != "" {
		t.Fatalf("%+v", list)
	}
	if out := h.rpc(t, "resources/list", map[string]string{"cursor": "x"}); out.Error == nil {
		t.Fatal("bad cursor")
	}
	if out := h.rpc(t, "resources/list", map[string]string{"cursor": "9:0"}); out.Error != nil {
		t.Fatal("past the end is empty")
	}
	if out := h.rpc(t, "resources/templates/list", nil); !strings.Contains(string(out.Result), itemTemplate) {
		t.Fatal(string(out.Result))
	}
	for uri, want := range map[string]string{URIQueue: "Show.S01", URIHealth: "integrations", "corsarr://item/movie/1": "Alpha", "corsarr://item/music/1": "Band"} {
		if out := h.rpc(t, "resources/read", map[string]string{"uri": uri}); out.Error != nil || !strings.Contains(string(out.Result), want) {
			t.Errorf("%s: %s %v", uri, out.Result, out.Error)
		}
	}
	for _, uri := range []string{"corsarr://nope", "corsarr://item/movie/x", "corsarr://item/movie/9"} {
		if out := h.rpc(t, "resources/read", map[string]string{"uri": uri}); out.Error == nil || out.Error.Code != codeNotFound {
			t.Errorf("%s: %+v", uri, out.Error)
		}
	}
	if out := h.rpc(t, "resources/subscribe", map[string]string{"uri": "corsarr://nope"}); out.Error == nil {
		t.Fatal("unknown uri")
	}
	h.rpc(t, "resources/subscribe", map[string]string{"uri": "corsarr://item/tv/1"})
	if !h.sess.subscribed("corsarr://item/tv/1") {
		t.Fatal("subscribed")
	}
	h.rpc(t, "resources/unsubscribe", map[string]string{"uri": "corsarr://item/tv/1"})
	if h.sess.subscribed("corsarr://item/tv/1") {
		t.Fatal("unsubscribed")
	}
	if out := h.rpc(t, "resources/read", "x"); out.Error == nil {
		t.Fatal("bad params")
	}
	h.mv.err = errBoom
	if out := h.rpc(t, "resources/list", nil); out.Error == nil {
		t.Fatal("list errors surface")
	}
}

func TestEventsMapToResources(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	items := []domain.EventItem{
		{Ref: domain.ItemRef{Kind: domain.KindMovie, ID: 1}}, {Ref: domain.ItemRef{Kind: domain.KindTV, ID: 12}},
		{Ref: domain.ItemRef{Kind: domain.KindTV, ID: 1, Variant: domain.VariantSeries}}, {Ref: domain.ItemRef{Kind: domain.KindMusic, ID: 100}},
		{Ref: domain.ItemRef{Kind: domain.KindMusic, ID: 1, Variant: domain.VariantArtist}}, {Ref: domain.ItemRef{Kind: domain.KindTV, ID: 999}},
		{Ref: domain.ItemRef{Kind: domain.KindMusic, ID: 5, Variant: domain.VariantTrack}},
	}
	got := h.s.urisOf(ctx, domain.Event{Type: domain.EvImportCompleted, Items: items})
	want := "corsarr://queue corsarr://item/movie/1 corsarr://item/tv/1 corsarr://item/tv/1 corsarr://item/music/1 corsarr://item/music/1"
	if strings.Join(got, " ") != want {
		t.Fatal(got)
	}
	if got := h.s.urisOf(ctx, domain.Event{Type: domain.EvItemUpdated}); len(got) != 0 {
		t.Fatal(got)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	h.s.Run(ctx) // returns once the context ends
}
