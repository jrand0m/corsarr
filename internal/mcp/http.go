package mcp

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/config"
)

// Streamable HTTP headers.
const (
	headerSession  = "Mcp-Session-Id"
	headerProtocol = "Mcp-Protocol-Version"
	maxBody        = 4 << 20
)

// KeepAlive is the comment interval on a notification stream.
var KeepAlive = 25 * time.Second

// Mount registers /mcp on the main listener; it answers 404 until
// mcp.enabled is set.
func (s *Service) Mount(m api.Mounter) {
	if sm, ok := m.(api.ScopedMounter); ok {
		sm.HandleScoped("/mcp", s.ServeHTTP)
	}
}

// ServeHTTP is the Streamable HTTP transport: POST carries one JSON-RPC
// message, GET opens the session's notification stream, DELETE ends it.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request, scope api.KeyScope) {
	cfg := s.cfg()
	switch {
	case !cfg.Enabled:
		http.NotFound(w, r)
	case !originAllowed(r, cfg):
		writeRPCError(w, http.StatusForbidden, rpcErr(codeInvalidRequest, "origin %q is not allowed (mcp.allowedOrigins)", r.Header.Get("Origin")))
	case scope == api.ScopeNone:
		w.Header().Set("WWW-Authenticate", `Bearer realm="corsarr"`)
		writeRPCError(w, http.StatusUnauthorized, rpcErr(codeInvalidRequest, "send the corsarr API key as a Bearer token or in X-Api-Key"))
	case !protocolSupported(r.Header.Get(headerProtocol)):
		writeRPCError(w, http.StatusBadRequest, rpcErr(codeInvalidRequest, "unsupported %s %q", headerProtocol, r.Header.Get(headerProtocol)))
	default:
		s.serveMethod(w, r, keyOf(scope))
	}
}

func (s *Service) serveMethod(w http.ResponseWriter, r *http.Request, key Key) {
	switch r.Method {
	case http.MethodPost:
		s.post(w, r, key)
	case http.MethodGet:
		s.stream(w, r, key)
	case http.MethodDelete:
		s.end(w, r, key)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func keyOf(scope api.KeyScope) Key {
	if scope == api.ScopeFull {
		return KeyFull
	}
	return KeyRead
}

func protocolSupported(v string) bool { return v == "" || slices.Contains(ProtocolVersions, v) }

// originAllowed admits clients without an Origin (not a browser), the
// server's own origin and the configured ones; anything else could be a page
// driving a local browser against the server (DNS rebinding).
func originAllowed(r *http.Request, cfg config.MCP) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || slices.Contains(cfg.AllowedOrigins, origin) {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

func writeRPCError(w http.ResponseWriter, status int, e *rpcError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(errorResponse(e))
}

func (s *Service) post(w http.ResponseWriter, r *http.Request, key Key) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeRPCError(w, http.StatusRequestEntityTooLarge, rpcErr(codeInvalidRequest, "message too large"))
		return
	}
	req, perr := parseRequest(body)
	if perr != nil {
		writeRPCError(w, http.StatusBadRequest, perr)
		return
	}
	sess, status := s.postSession(r, req, key)
	if sess == nil {
		writeRPCError(w, status, rpcErr(codeInvalidRequest, "%s", http.StatusText(status)+": start with initialize and send "+headerSession))
		return
	}
	out := s.Handle(r.Context(), sess, body)
	if out == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set(headerSession, sess.ID)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

// postSession starts a session on initialize and finds it otherwise: a
// missing id is 400, an unknown or ended one 404 (the client starts over).
func (s *Service) postSession(r *http.Request, req request, key Key) (*Session, int) {
	if req.Method == "initialize" {
		return s.NewSession(key), 0
	}
	return s.lookup(r, key)
}

func (s *Service) lookup(r *http.Request, key Key) (*Session, int) {
	id := r.Header.Get(headerSession)
	if id == "" {
		return nil, http.StatusBadRequest
	}
	sess, ok := s.session(id, key)
	if !ok {
		return nil, http.StatusNotFound
	}
	return sess, 0
}

// stream sends the session's notifications as server-sent events until the
// client goes away.
func (s *Service) stream(w http.ResponseWriter, r *http.Request, key Key) {
	sess, status := s.lookup(r, key)
	if sess == nil {
		writeRPCError(w, status, rpcErr(codeInvalidRequest, "%s", http.StatusText(status)))
		return
	}
	if !sess.openStream() {
		writeRPCError(w, http.StatusConflict, rpcErr(codeInvalidRequest, "this session already has a stream"))
		return
	}
	defer sess.closeStream()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flush(w)
	tick := time.NewTicker(KeepAlive)
	defer tick.Stop()
	for s.relay(w, r, sess, tick.C) {
	}
}

// relay writes one notification or keep-alive; false ends the stream.
func (s *Service) relay(w http.ResponseWriter, r *http.Request, sess *Session, tick <-chan time.Time) bool {
	var err error
	select {
	case <-r.Context().Done():
		return false
	case msg := <-sess.out:
		_, err = fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
	case <-tick:
		_, err = io.WriteString(w, ": keep-alive\n\n")
	}
	flush(w)
	return err == nil
}

func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Service) end(w http.ResponseWriter, r *http.Request, key Key) {
	sess, status := s.lookup(r, key)
	if sess == nil {
		w.WriteHeader(status)
		return
	}
	s.CloseSession(sess.ID)
	w.WriteHeader(http.StatusNoContent)
}
