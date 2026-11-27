package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Key says which credential a session uses; the rate limit is per key.
type Key string

const (
	KeyFull  Key = "apikey"   // the corsarr API key
	KeyRead  Key = "readonly" // the read-only key
	KeyLocal Key = "stdio"    // a local process on stdio
)

// maxSessions bounds the HTTP sessions kept; the least recently used goes.
const maxSessions = 64

// Session is one client connection.
type Session struct {
	ID  string
	key Key
	out chan []byte // server → client notifications

	mu       sync.Mutex
	client   string
	ready    bool
	subs     map[string]bool
	used     time.Time
	streamed bool
}

func newSession(key Key, now time.Time) *Session {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return &Session{ID: hex.EncodeToString(b), key: key, out: make(chan []byte, 32), subs: map[string]bool{}, used: now}
}

func (s *Session) setClient(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client = name
}

// Client is the name the client gave at initialize.
func (s *Session) Client() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == "" {
		return "unknown"
	}
	return s.client
}

func (s *Session) setReady() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = true
}

func (s *Session) setSub(uri string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on {
		s.subs[uri] = true
		return
	}
	delete(s.subs, uri)
}

func (s *Session) subscribed(uri string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subs[uri]
}

// notify queues a notification; a full queue drops it (the client re-reads).
func (s *Session) notify(msg []byte) {
	select {
	case s.out <- msg:
	default:
	}
}

// openStream claims the session's one notification stream.
func (s *Session) openStream() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streamed {
		return false
	}
	s.streamed = true
	return true
}

func (s *Session) closeStream() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamed = false
}

func (s *Session) touch(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.used = now
}

func (s *Session) lastUsed() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.used
}

// ── session registry ─────────────────────────────────────────────────────────

// NewSession starts a session for a key and keeps it until Close or until
// it is the least recently used of too many.
func (s *Service) NewSession(key Key) *Session {
	sess := newSession(key, s.d.Now())
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) >= maxSessions {
		s.evictLocked()
	}
	s.sessions[sess.ID] = sess
	return sess
}

func (s *Service) evictLocked() {
	var oldest *Session
	for _, x := range s.sessions {
		if oldest == nil || x.lastUsed().Before(oldest.lastUsed()) {
			oldest = x
		}
	}
	delete(s.sessions, oldest.ID)
}

// session finds a live session of the key.
func (s *Service) session(id string, key Key) (*Session, bool) {
	s.mu.Lock()
	sess, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok || sess.key != key {
		return nil, false
	}
	sess.touch(s.d.Now())
	return sess, true
}

// CloseSession forgets a session.
func (s *Service) CloseSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *Service) eachSession(fn func(*Session)) {
	s.mu.Lock()
	list := make([]*Session, 0, len(s.sessions))
	for _, x := range s.sessions {
		list = append(list, x)
	}
	s.mu.Unlock()
	for _, x := range list {
		fn(x)
	}
}

// ── rate limit ───────────────────────────────────────────────────────────────

// limiter is a token bucket per key refilled at perMinute per minute.
type limiter struct {
	mu      sync.Mutex
	now     func() time.Time
	buckets map[Key]*bucket
}

type bucket struct {
	tokens float64
	at     time.Time
}

func newLimiter(now func() time.Time) *limiter {
	return &limiter{now: now, buckets: map[Key]*bucket{}}
}

// take spends one token of key; perMinute 0 means unlimited.
func (l *limiter) take(key Key, perMinute int) bool {
	if perMinute <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now, max := l.now(), float64(perMinute)
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: max, at: now}
		l.buckets[key] = b
	}
	b.tokens = min(max, b.tokens+now.Sub(b.at).Minutes()*max)
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (s *Service) allow(sess *Session) bool { return s.limit.take(sess.key, s.cfg().RateLimit) }
