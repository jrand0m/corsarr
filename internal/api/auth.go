package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/store"
)

// SessionCookie is the session cookie name.
const SessionCookie = "corsarr_session"

// SessionTTL is how long a login lasts.
const SessionTTL = 30 * 24 * time.Hour

// MinPasswordLen is the minimum admin password length.
const MinPasswordLen = 8

// RandomToken returns n random bytes, base64url.
func RandomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand never fails on supported platforms
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// EnsureAPIKey generates the API key on first start; it is
// idempotent.
func EnsureAPIKey(ctx context.Context, a AuthStore) error {
	if _, ok, err := a.APIKey(ctx); ok || err != nil {
		return err
	}
	return a.SetAPIKey(ctx, RandomToken(24))
}

// authMethod says how a request authenticated ("" = not).
func (s *Server) authMethod(r *http.Request) string {
	switch {
	case s.validAPIKey(r):
		return "apikey"
	case s.validSession(r):
		return "session"
	case s.d.AuthMode == "local" && privateRemote(r.RemoteAddr):
		return "local"
	case s.d.AuthMode == "proxy" && proxyUser(r) != "":
		return "proxy"
	}
	return ""
}

func (s *Server) validAPIKey(r *http.Request) bool {
	got := r.Header.Get("X-Api-Key")
	if got == "" {
		got = r.URL.Query().Get("apikey") // Torznab / iCal compatibility
	}
	want := *s.apiKey.Load()
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(SessionCookie)
	if err != nil || c.Value == "" {
		return false
	}
	ok, err := s.d.Auth.SessionValid(r.Context(), hashToken(c.Value), s.d.Now())
	return ok && err == nil
}

func proxyUser(r *http.Request) string {
	if u := r.Header.Get("Remote-User"); u != "" {
		return u
	}
	return r.Header.Get("X-Forwarded-User")
}

func privateRemote(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

// requireAuth admits authenticated requests; before first run it answers
// setup_required so the UI shows only the first-run screen.
func (s *Server) requireAuth(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authMethod(r) != "" {
			h(w, r)
			return
		}
		if _, ok, _ := s.d.Auth.Admin(r.Context()); !ok && s.d.AuthMode == "password" {
			WriteError(w, http.StatusUnauthorized, "setup_required", "set the admin password first", "")
			return
		}
		WriteError(w, http.StatusUnauthorized, "unauthorized", "sign in or send X-Api-Key", "")
	})
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) firstRun(w http.ResponseWriter, r *http.Request) {
	_, ok, err := s.d.Auth.Admin(r.Context())
	if err != nil {
		s.internal(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"required": !ok, "authMode": s.d.AuthMode})
}

// setup is the one-time first-run transition: it sets the admin password and
// signs the caller in.
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !DecodeJSON(w, r, &c) || !validCredentials(w, c) {
		return
	}
	hash, err := HashPassword(c.Password)
	if err != nil {
		s.internal(w, err)
		return
	}
	if err := s.d.Auth.SetAdmin(r.Context(), c.Username, hash); errors.Is(err, store.ErrExists) {
		WriteError(w, http.StatusConflict, "already_set_up", "the admin user already exists; sign in", "")
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	s.startSession(w, r)
}

func validCredentials(w http.ResponseWriter, c credentials) bool {
	if strings.TrimSpace(c.Username) == "" {
		WriteError(w, http.StatusBadRequest, "invalid", "username is required", "username")
		return false
	}
	if len(c.Password) < MinPasswordLen {
		WriteError(w, http.StatusBadRequest, "invalid", "password must have at least 8 characters", "password")
		return false
	}
	return true
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !DecodeJSON(w, r, &c) {
		return
	}
	u, ok, err := s.d.Auth.Admin(r.Context())
	if err != nil {
		s.internal(w, err)
		return
	}
	if !ok || u.Username != c.Username || !VerifyPassword(u.PasswordHash, c.Password) {
		WriteError(w, http.StatusUnauthorized, "invalid_credentials", "wrong username or password", "")
		return
	}
	s.startSession(w, r)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request) {
	token := RandomToken(32)
	exp := s.d.Now().Add(SessionTTL)
	if err := s.d.Auth.CreateSession(r.Context(), hashToken(token), exp); err != nil {
		s.internal(w, err)
		return
	}
	http.SetCookie(w, s.cookie(r, token, exp))
	WriteJSON(w, http.StatusOK, map[string]any{"authenticated": true})
}

func (s *Server) cookie(r *http.Request, value string, exp time.Time) *http.Cookie {
	return &http.Cookie{
		Name: SessionCookie, Value: value, Path: strings.TrimSuffix(s.d.BasePath, "/") + "/", Expires: exp,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	}
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		if err := s.d.Auth.DeleteSession(r.Context(), hashToken(c.Value)); err != nil {
			s.internal(w, err)
			return
		}
	}
	http.SetCookie(w, s.cookie(r, "", time.Unix(0, 0)))
	WriteJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	m := s.authMethod(r)
	WriteJSON(w, http.StatusOK, map[string]any{"authenticated": m != "", "method": m})
}

func (s *Server) getAPIKey(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"apiKey": *s.apiKey.Load()})
}

func (s *Server) rotateAPIKey(w http.ResponseWriter, r *http.Request) {
	key := RandomToken(24)
	if err := s.d.Auth.SetAPIKey(r.Context(), key); err != nil {
		s.internal(w, err)
		return
	}
	s.apiKey.Store(&key)
	WriteJSON(w, http.StatusOK, map[string]string{"apiKey": key})
}
