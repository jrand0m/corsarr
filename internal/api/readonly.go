package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

// ReadOnlyKeyStore keeps the second, read-only API key (store.Auth
// implements it). It only admits assistant (MCP) clients to read tools; it
// never opens /api/v1.
type ReadOnlyKeyStore interface {
	ReadOnlyAPIKey(ctx context.Context) (string, bool, error)
	SetReadOnlyAPIKey(ctx context.Context, key string) error
}

func (s *Server) roStore() (ReadOnlyKeyStore, bool) {
	ro, ok := s.d.Auth.(ReadOnlyKeyStore)
	return ro, ok
}

func (s *Server) loadReadOnlyKey(ctx context.Context) error {
	key := ""
	if ro, ok := s.roStore(); ok {
		k, _, err := ro.ReadOnlyAPIKey(ctx)
		if err != nil {
			return err
		}
		key = k
	}
	s.roKey.Store(&key)
	return nil
}

// presentedKey is the key in X-Api-Key or an Authorization Bearer token.
func presentedKey(r *http.Request) string {
	if k := r.Header.Get("X-Api-Key"); k != "" {
		return k
	}
	scheme, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	if strings.EqualFold(scheme, "Bearer") {
		return strings.TrimSpace(token)
	}
	return ""
}

func sameKey(got, want string) bool {
	return got != "" && want != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// KeyScope classifies the key a request presents in a header.
func (s *Server) KeyScope(r *http.Request) KeyScope {
	got := presentedKey(r)
	switch {
	case sameKey(got, *s.apiKey.Load()):
		return ScopeFull
	case sameKey(got, *s.roKey.Load()):
		return ScopeRead
	}
	return ScopeNone
}

// getReadOnlyKey returns the read-only key, issuing one on first use.
func (s *Server) getReadOnlyKey(w http.ResponseWriter, r *http.Request) {
	if key := *s.roKey.Load(); key != "" {
		WriteJSON(w, http.StatusOK, map[string]string{"apiKey": key})
		return
	}
	s.rotateReadOnlyKey(w, r)
}

func (s *Server) rotateReadOnlyKey(w http.ResponseWriter, r *http.Request) {
	s.setReadOnlyKey(w, r, RandomToken(24))
}

func (s *Server) revokeReadOnlyKey(w http.ResponseWriter, r *http.Request) {
	s.setReadOnlyKey(w, r, "")
}

func (s *Server) setReadOnlyKey(w http.ResponseWriter, r *http.Request, key string) {
	ro, ok := s.roStore()
	if !ok {
		WriteError(w, http.StatusNotImplemented, "unsupported", "read-only keys are not available", "")
		return
	}
	if err := ro.SetReadOnlyAPIKey(r.Context(), key); err != nil {
		s.internal(w, err)
		return
	}
	s.roKey.Store(&key)
	WriteJSON(w, http.StatusOK, map[string]string{"apiKey": key})
}
