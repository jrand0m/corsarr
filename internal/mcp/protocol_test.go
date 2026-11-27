package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
)

func TestInitializeNegotiatesAndAnnouncesCapabilities(t *testing.T) {
	h := newHarness(t)
	out := h.rpc(t, "initialize", map[string]any{"protocolVersion": "2025-06-18", "clientInfo": map[string]string{"name": "desk"}})
	var r struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      map[string]any `json:"serverInfo"`
	}
	_ = json.Unmarshal(out.Result, &r)
	if r.ProtocolVersion != "2025-06-18" || r.Capabilities["tools"] == nil || r.Capabilities["resources"] == nil || r.ServerInfo["name"] != "corsarr" {
		t.Fatalf("%s", out.Result)
	}
	if h.sess.Client() != "desk" {
		t.Fatal(h.sess.Client())
	}
	out = h.rpc(t, "initialize", map[string]any{"protocolVersion": "1999-01-01"})
	_ = json.Unmarshal(out.Result, &r)
	if r.ProtocolVersion != ProtocolVersions[0] {
		t.Fatal("unknown revisions get the newest")
	}
	if out := h.rpc(t, "initialize", "not an object"); out.Error == nil || out.Error.Code != codeInvalidParams {
		t.Fatal("bad params")
	}
}

func TestMessagesThatGetNoAnswerOrAnError(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if out := h.s.Handle(ctx, h.sess, []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); out != nil || !h.sess.ready {
		t.Fatal("notifications get no answer")
	}
	if out := h.s.Handle(ctx, h.sess, []byte(`{"jsonrpc":"2.0","id":4,"result":{}}`)); out != nil {
		t.Fatal("responses get no answer")
	}
	for raw, code := range map[string]int{
		`{nope`: codeParse, `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`: codeInvalidRequest,
		`{"jsonrpc":"1.0","id":1,"method":"ping"}`: codeInvalidRequest, `{"jsonrpc":"2.0","id":1,"method":"frob"}`: codeMethodNotFound,
	} {
		var out rpcOut
		_ = json.Unmarshal(h.s.Handle(ctx, h.sess, []byte(raw)), &out)
		if out.Error == nil || out.Error.Code != code {
			t.Errorf("%s: %+v", raw, out.Error)
		}
	}
	if out := h.rpc(t, "ping", nil); string(out.Result) != "{}" {
		t.Fatal(string(out.Result))
	}
}

func toolNames(t *testing.T, h *harness, sess *Session) []string {
	t.Helper()
	var r struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
			Annotations map[string]any `json:"annotations"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(h.rpcOn(t, sess, "tools/list", nil).Result, &r)
	var out []string
	for _, tl := range r.Tools {
		if tl.InputSchema["type"] != "object" || tl.Annotations["readOnlyHint"] == nil {
			t.Errorf("%s: schema or annotations missing", tl.Name)
		}
		out = append(out, tl.Name)
	}
	return out
}

func TestToolListFollowsScopeAndAllowList(t *testing.T) {
	h := newHarness(t)
	if n := len(toolNames(t, h, h.sess)); n != len(h.s.Tools()) {
		t.Fatalf("write scope lists every tool: %d of %d", n, len(h.s.Tools()))
	}
	ro := h.s.NewSession(KeyRead)
	for _, name := range toolNames(t, h, ro) {
		if h.s.byName[name].Write {
			t.Errorf("read-only key lists %s", name)
		}
	}
	h.setMCP(config.MCP{Enabled: true, Scope: config.MCPScopeRead})
	if names := toolNames(t, h, h.sess); strings.Contains(strings.Join(names, ","), "request_movie") {
		t.Fatal("read scope hides write tools")
	}
	h.setMCP(config.MCP{Enabled: true, Scope: config.MCPScopeWrite, Tools: []string{"get_health", "request_movie"}})
	if names := toolNames(t, h, h.sess); strings.Join(names, ",") != "request_movie,get_health" {
		t.Fatal(names)
	}
	if out := h.rpc(t, "tools/call", map[string]any{"name": "find_media"}); out.Error == nil || out.Error.Code != codeInvalidParams {
		t.Fatal("a tool outside the allow-list is unknown")
	}
	if out := h.rpc(t, "tools/call", "x"); out.Error == nil {
		t.Fatal("bad params")
	}
}

// Every tool has a well-formed schema and hints consistent with its scope.
func TestToolSchemasAndHints(t *testing.T) {
	h := newHarness(t)
	for _, tl := range h.s.Tools() {
		info := tl.info()
		b, err := json.Marshal(info)
		if err != nil || info.InputSchema["type"] != "object" || info.Description == "" || info.Title == "" {
			t.Errorf("%s: %v %s", tl.Name, err, b)
		}
		if info.Annotations.ReadOnlyHint == tl.Write || (tl.Destructive && !tl.Write) {
			t.Errorf("%s: hints %+v", tl.Name, info.Annotations)
		}
		if props, _ := info.InputSchema["properties"].(map[string]schema); props == nil {
			t.Errorf("%s: no properties", tl.Name)
		}
	}
	if !h.s.byName["cancel_request"].Destructive || h.s.byName["start_search"].Idempotent {
		t.Fatal("hints")
	}
}

func TestWriteToolsRefusedWithoutWriteScope(t *testing.T) {
	h := newHarness(t)
	ro := h.s.NewSession(KeyRead)
	sub := h.bus.Subscribe(16, func(t domain.EventType) bool { return t == domain.EvMCPCalled })
	out := h.callOn(t, ro, "request_movie", map[string]any{"tmdbId": 99})
	if !out.IsError || !strings.Contains(out.Text, "write-scoped") || h.c.has("movies.add") {
		t.Fatalf("%+v", out)
	}
	ev := next(t, sub)
	if d := ev.Data.(CallData); d.Outcome != outcomeRefused || d.Key != KeyRead {
		t.Fatalf("%+v", d)
	}
	h.setMCP(config.MCP{Enabled: true, Scope: config.MCPScopeRead})
	if out := h.call(t, "set_monitored", map[string]any{"kind": "movie", "id": 1, "monitored": false}); !out.IsError {
		t.Fatal("mcp.scope read refuses the full key too")
	}
}

func next(t *testing.T, sub interface {
	Next(context.Context) (domain.Event, error)
}) domain.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ev, err := sub.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestCallsAreAuditedWithSecretsRedacted(t *testing.T) {
	h := newHarness(t)
	h.rpc(t, "initialize", map[string]any{"clientInfo": map[string]string{"name": "desk"}})
	sub := h.bus.Subscribe(16, func(t domain.EventType) bool { return t == domain.EvMCPCalled })
	h.call(t, "get_history", map[string]any{"limit": 5, "apiKey": "s3cret"})
	ev := next(t, sub)
	d := ev.Data.(CallData)
	if ev.Actor != "mcp:desk" || d.Tool != "get_history" || d.Outcome != outcomeOK || d.Arguments["apiKey"] != "[redacted]" || d.Arguments["limit"] != float64(5) {
		t.Fatalf("%+v %+v", ev, d)
	}
	h.call(t, "request_status", map[string]any{"kind": "movie", "id": 404})
	if d := next(t, sub).Data.(CallData); d.Outcome != outcomeError || d.Error == "" {
		t.Fatalf("%+v", d)
	}
	h.modes[domain.KindMovie] = domain.ModeDisabled
	h.call(t, "request_movie", map[string]any{"tmdbId": 1})
	if d := next(t, sub).Data.(CallData); d.Outcome != outcomeRefused || !strings.Contains(d.Error, "mediaKinds.movie.mode") {
		t.Fatalf("%+v", d)
	}
	if redact(json.RawMessage(`[1]`)) != nil {
		t.Fatal("non-object arguments")
	}
}

func TestRateLimitPerKey(t *testing.T) {
	h := newHarness(t)
	h.setMCP(config.MCP{Enabled: true, Scope: config.MCPScopeWrite, RateLimit: 2})
	ro := h.s.NewSession(KeyRead)
	for i := 0; i < 2; i++ {
		h.call(t, "list_root_folders", nil)
	}
	if out := h.rpc(t, "tools/call", map[string]any{"name": "list_root_folders"}); out.Error == nil || out.Error.Code != codeRateLimited {
		t.Fatal("third call in a minute is limited")
	}
	if out := h.rpc(t, "resources/read", map[string]any{"uri": URIHealth}); out.Error == nil {
		t.Fatal("reads count too")
	}
	if out := h.callOn(t, ro, "list_root_folders", nil); out.IsError {
		t.Fatal("another key has its own budget")
	}
	h.now = h.now.Add(31 * time.Second)
	if out := h.call(t, "list_root_folders", nil); out.IsError {
		t.Fatal("the bucket refills")
	}
	if !newLimiter(time.Now).take(KeyFull, 0) {
		t.Fatal("0 is unlimited")
	}
}

func TestSessionsAreBoundedAndBoundToTheirKey(t *testing.T) {
	h := newHarness(t)
	first := h.sess
	for i := 0; i < maxSessions; i++ {
		h.now = h.now.Add(time.Second)
		h.s.NewSession(KeyFull)
	}
	if _, ok := h.s.session(first.ID, KeyFull); ok {
		t.Fatal("the least recently used session is evicted")
	}
	s := h.s.NewSession(KeyFull)
	if _, ok := h.s.session(s.ID, KeyRead); ok {
		t.Fatal("another key cannot use the session")
	}
	h.s.CloseSession(s.ID)
	if _, ok := h.s.session(s.ID, KeyFull); ok {
		t.Fatal("closed")
	}
	if (&Session{}).Client() != "unknown" {
		t.Fatal("client name")
	}
	full := newSession(KeyFull, day)
	for i := 0; i < cap(full.out)+2; i++ {
		full.notify([]byte("x")) // never blocks
	}
}
