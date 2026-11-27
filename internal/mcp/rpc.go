package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
)

// ProtocolVersions are the MCP revisions this server speaks, newest first.
var ProtocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
	codeRateLimited    = -32000
	codeNotFound       = -32002 // resource not found
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// notification reports whether no answer is expected (notifications, and
// responses to requests this server never sends).
func (r request) notification() bool { return len(r.ID) == 0 || r.Method == "" }

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

func rpcErr(code int, format string, args ...any) *rpcError {
	return &rpcError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// errorResponse is an answer to a message that could not be read.
func errorResponse(e *rpcError) []byte {
	b, _ := json.Marshal(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: e})
	return b
}

func parseRequest(raw []byte) (request, *rpcError) {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		if len(raw) > 0 && raw[0] == '[' {
			return req, rpcErr(codeInvalidRequest, "batches are not supported; send one message per request")
		}
		return req, rpcErr(codeParse, "invalid JSON: %v", err)
	}
	if req.JSONRPC != "2.0" {
		return req, rpcErr(codeInvalidRequest, `jsonrpc must be "2.0"`)
	}
	return req, nil
}

// Handle answers one JSON-RPC message of a session; nil means no answer
// (notifications and responses).
func (s *Service) Handle(ctx context.Context, sess *Session, raw []byte) []byte {
	req, perr := parseRequest(raw)
	if perr != nil {
		return errorResponse(perr)
	}
	if req.notification() {
		s.notified(sess, req)
		return nil
	}
	result, err := s.dispatch(ctx, sess, req)
	resp := response{JSONRPC: "2.0", ID: req.ID, Result: result}
	if err != nil {
		var re *rpcError
		if !errors.As(err, &re) {
			re = rpcErr(codeInternal, "%v", err)
		}
		resp.Result, resp.Error = nil, re
	}
	b, _ := json.Marshal(resp)
	return b
}

func (s *Service) notified(sess *Session, req request) {
	if req.Method == "notifications/initialized" {
		sess.setReady()
	}
}

type handler func(ctx context.Context, sess *Session, params json.RawMessage) (any, error)

func (s *Service) methods() map[string]handler {
	return map[string]handler{
		"initialize":               s.initialize,
		"ping":                     func(context.Context, *Session, json.RawMessage) (any, error) { return struct{}{}, nil },
		"tools/list":               s.listTools,
		"tools/call":               s.callTool,
		"resources/list":           s.listResources,
		"resources/templates/list": s.listTemplates,
		"resources/read":           s.readResource,
		"resources/subscribe":      s.subscribe,
		"resources/unsubscribe":    s.unsubscribe,
	}
}

func (s *Service) dispatch(ctx context.Context, sess *Session, req request) (any, error) {
	h, ok := s.methods()[req.Method]
	if !ok {
		return nil, rpcErr(codeMethodNotFound, "method %q is not supported", req.Method)
	}
	return h(ctx, sess, req.Params)
}

func decodeParams(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return rpcErr(codeInvalidParams, "invalid params: %v", err)
	}
	return nil
}

// ── initialize ───────────────────────────────────────────────────────────────

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

// negotiate answers the client's revision when supported, else the newest.
func negotiate(asked string) string {
	if slices.Contains(ProtocolVersions, asked) {
		return asked
	}
	return ProtocolVersions[0]
}

const instructions = "corsarr manages a media library (movies, TV, music). Use find_media to find something, " +
	"show the person the candidate, then request it with request_movie, request_show or request_music and " +
	"follow it with request_status. Lists are paged: pass nextCursor back as cursor."

func (s *Service) initialize(_ context.Context, sess *Session, params json.RawMessage) (any, error) {
	var p initializeParams
	if err := decodeParams(params, &p); err != nil {
		return nil, err
	}
	sess.setClient(p.ClientInfo.Name)
	return map[string]any{
		"protocolVersion": negotiate(p.ProtocolVersion),
		"capabilities": map[string]any{
			"tools":     map[string]bool{"listChanged": false},
			"resources": map[string]bool{"subscribe": true, "listChanged": false},
		},
		"serverInfo":   map[string]string{"name": "corsarr", "title": "corsarr", "version": s.d.Version},
		"instructions": instructions,
	}, nil
}

// ── tools ────────────────────────────────────────────────────────────────────

// exposed reports whether the allow-list lets a tool through.
func exposed(cfg config.MCP, name string) bool {
	return len(cfg.Tools) == 0 || slices.Contains(cfg.Tools, name)
}

// canWrite reports whether a session may call write tools now.
func (s *Service) canWrite(sess *Session) bool {
	return sess.key != KeyRead && s.cfg().Scope == config.MCPScopeWrite
}

func (s *Service) listTools(_ context.Context, sess *Session, _ json.RawMessage) (any, error) {
	cfg, write := s.cfg(), s.canWrite(sess)
	out := []toolInfo{}
	for _, t := range s.tools {
		if exposed(cfg, t.Name) && (write || !t.Write) {
			out = append(out, t.info())
		}
	}
	return map[string]any{"tools": out}, nil
}

type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Service) callTool(ctx context.Context, sess *Session, params json.RawMessage) (any, error) {
	var p callParams
	if err := decodeParams(params, &p); err != nil {
		return nil, err
	}
	t, ok := s.byName[p.Name]
	if !ok || !exposed(s.cfg(), p.Name) {
		return nil, rpcErr(codeInvalidParams, "unknown tool %q", p.Name)
	}
	if !s.allow(sess) {
		s.audit(sess, p, outcomeLimited, nil)
		return nil, rpcErr(codeRateLimited, "rate limit reached: at most %d calls per minute", s.cfg().RateLimit)
	}
	if t.Write && !s.canWrite(sess) {
		err := errors.New("refused: " + p.Name + " changes the library and needs a write-scoped API key with mcp.scope: write")
		s.audit(sess, p, outcomeRefused, err)
		return toolResult(Result{}, err), nil
	}
	res, err := t.Run(ctx, p.Arguments)
	s.audit(sess, p, outcomeOf(err), err)
	return toolResult(res, err), nil
}

// toolResult renders a tool's answer; failures are tool errors the model
// can read and correct.
func toolResult(res Result, err error) map[string]any {
	if err != nil {
		return map[string]any{"content": []map[string]string{{"type": "text", "text": errText(err)}}, "isError": true}
	}
	out := map[string]any{"content": []map[string]string{{"type": "text", "text": res.Text}}}
	if res.Data != nil {
		out["structuredContent"] = res.Data
	}
	return out
}

// errText explains a failure; a kind's mode reads as the reason.
func errText(err error) string {
	var me domain.ModeError
	if errors.As(err, &me) {
		return fmt.Sprintf("refused: %s is %s in corsarr (mediaKinds.%s.mode), so nothing is changed for it", me.Kind, me.Mode, me.Kind)
	}
	return err.Error()
}
