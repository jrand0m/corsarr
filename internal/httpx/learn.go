package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Learning switches on rate-limit learning: per service, the client finds
// where throttling starts (announced limits first, then throttles it meets)
// and keeps its pace a headroom below that. Now and then, only while real
// requests are queued, it checks with small steps whether an observed limit
// moved, stopping at the first throttle. Configured Limits always apply on
// top: learning only ever adds delay.
type Learning struct {
	Enabled    bool
	Headroom   float64       // share of a learned limit to use; 0 = 0.85
	ProbeEvery time.Duration // how often an observed limit is checked; 0 = 168h, negative = never
	Store      LimitStore    // keeps what was learned across restarts; optional
}

// LimitStore keeps one opaque document per service.
type LimitStore interface {
	LoadRateLimits(ctx context.Context) (map[string][]byte, error)
	SaveRateLimit(ctx context.Context, key string, doc []byte) error
}

// ErrHeld is returned instead of sending when a service asked corsarr to
// wait longer than the client waits on a caller's behalf.
type ErrHeld struct{ Until time.Time }

func (e ErrHeld) Error() string {
	return fmt.Sprintf("rate limited: the service asked to wait until %s", e.Until.UTC().Format(time.RFC3339))
}

// learning is the client's set of learners.
type learning struct {
	cfg   learnCfg
	store LimitStore
	mu    sync.Mutex
	all   map[string]*learner
}

func newLearning(o Learning) *learning {
	if !o.Enabled {
		return nil
	}
	cfg := learnCfg{headroom: o.Headroom, probeEvery: o.ProbeEvery}
	if cfg.headroom <= 0 || cfg.headroom > 1 {
		cfg.headroom = 0.85
	}
	if cfg.probeEvery == 0 {
		cfg.probeEvery = 7 * 24 * time.Hour
	}
	return &learning{cfg: cfg, store: o.Store, all: map[string]*learner{}}
}

// RefKey names a plugin instance's own limit, kept apart from its host's.
func RefKey(ref domain.IntegrationRef) string { return ref.Type + "/" + string(ref.Key) }

func learnKey(ref *domain.IntegrationRef, host string) string {
	if ref != nil {
		return RefKey(*ref)
	}
	return host
}

func (g *learning) get(key string) *learner {
	g.mu.Lock()
	defer g.mu.Unlock()
	l, ok := g.all[key]
	if !ok {
		l = newLearner(key)
		g.all[key] = l
	}
	return l
}

// doc is what is kept per service.
type doc struct {
	Ceilings     []Ceiling `json:"ceilings"`
	LastThrottle time.Time `json:"lastThrottle"`
	CheckedAt    time.Time `json:"checkedAt"`
	Challenged   bool      `json:"challenged,omitempty"`
}

func (l *learner) doc() doc {
	return doc{Ceilings: append([]Ceiling(nil), l.ceil...), LastThrottle: l.lastThrottle, CheckedAt: l.checkedAt, Challenged: l.challenged}
}

// save writes one service's record; a failed write only costs relearning.
func (g *learning) save(l *learner) {
	if g.store == nil {
		return
	}
	l.mu.Lock()
	b, _ := json.Marshal(l.doc()) // plain values always marshal
	l.mu.Unlock()
	_ = g.store.SaveRateLimit(context.Background(), l.key, b)
}

// RestoreLimits loads what was learned before the last restart.
func (c *Client) RestoreLimits(ctx context.Context) error {
	g := c.learn
	if g == nil || g.store == nil {
		return nil
	}
	docs, err := g.store.LoadRateLimits(ctx)
	if err != nil {
		return err
	}
	for key, b := range docs {
		var d doc
		if json.Unmarshal(b, &d) != nil {
			continue // an unreadable record is relearned
		}
		l := g.get(key)
		l.mu.Lock()
		l.ceil, l.lastThrottle, l.checkedAt, l.challenged = d.Ceilings, d.LastThrottle, d.CheckedAt, d.Challenged
		l.mu.Unlock()
	}
	return nil
}

// SetProbe sets how far checks of ref's limit may go (scraped sites with
// login or anti-bot pages are best left at ProbeOff).
func (c *Client) SetProbe(ref domain.IntegrationRef, m ProbeMode) {
	c.shape(RefKey(ref), func(l *learner) { l.mode = m })
}

// Cap tells the learner an owner's limit on ref (n requests per window); no
// learned ceiling and no check goes above it.
func (c *Client) Cap(ref domain.IntegrationRef, window time.Duration, n float64) {
	c.shape(RefKey(ref), func(l *learner) { l.caps[window] = n })
}

// Declare records a service's documented limit for ref; it is kept to like
// an announced one and never checked.
func (c *Client) Declare(ref domain.IntegrationRef, window time.Duration, n float64) {
	c.shape(RefKey(ref), func(l *learner) {
		if old, ok := l.ceiling(window); !ok || old.Source != SourceDeclared || old.Limit != n {
			l.setCeiling(Ceiling{Window: window, Limit: n, Source: SourceDeclared, At: c.o.Now()})
		}
	})
}

func (c *Client) shape(key string, f func(*learner)) {
	if c.learn == nil {
		return
	}
	l := c.learn.get(key)
	l.mu.Lock()
	f(l)
	l.mu.Unlock()
}

// Throttled reports a throttle a plugin saw inside an answer the client
// passed as fine (an API error code meaning "slow down").
func (c *Client) Throttled(ref domain.IntegrationRef, retryAfter time.Duration) {
	if c.learn == nil {
		return
	}
	l := c.learn.get(RefKey(ref))
	l.mu.Lock()
	l.throttled(c.o.Now(), retryAfter, retryAfter > 0, c.learn.cfg)
	l.mu.Unlock()
	c.learn.save(l)
}

// pace waits for the learner's next slot for this service. A hold longer
// than the client's longest wait fails fast instead of blocking the caller.
func (c *Client) pace(ctx context.Context, l *learner) error {
	if l == nil {
		return nil
	}
	now := c.o.Now()
	l.mu.Lock()
	if l.hold.Sub(now) > c.o.MaxBackoff {
		until := l.hold
		l.mu.Unlock()
		return ErrHeld{Until: until}
	}
	wait := l.reserve(now, c.learn.cfg)
	l.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	return c.o.Sleep(ctx, wait)
}

// observe feeds one answer to the service's learner.
func (c *Client) observe(l *learner, resp *http.Response) {
	if l == nil || resp == nil {
		return
	}
	now := c.o.Now()
	ad := parseAdvert(resp.Header, now)
	retry, hasRetry := parseRetryAfter(resp.Header.Get("Retry-After"), now)
	isChallenge := challenged(resp)
	l.mu.Lock()
	changed := l.advertised(ad, now)
	switch {
	case isChallenge:
		changed = changed || !l.challenged
		l.challenged = true
		l.probe = nil
	case throttle(resp.StatusCode, hasRetry):
		l.throttled(now, retry, hasRetry, c.learn.cfg)
		changed = true
	default:
		changed = l.accepted(now, c.learn.cfg) || changed
	}
	l.mu.Unlock()
	if changed {
		c.learn.save(l)
	}
}

// throttle: 429 always, 503 when it says when to come back.
func throttle(code int, hasRetry bool) bool {
	return code == http.StatusTooManyRequests || (code == http.StatusServiceUnavailable && hasRetry)
}

// LimitStatus is one service's learned limit, for display.
type LimitStatus struct {
	Service      string          `json:"service"`
	Ceilings     []CeilingStatus `json:"ceilings"`
	Recent       []WindowCount   `json:"recent"`
	Pace         float64         `json:"pace"` // requests per second allowed now; 0 = not paced
	Probe        string          `json:"probe"`
	Probing      bool            `json:"probing"`
	LastThrottle *time.Time      `json:"lastThrottle,omitempty"`
	CheckedAt    *time.Time      `json:"checkedAt,omitempty"`
	NextCheck    *time.Time      `json:"nextCheck,omitempty"`
	HeldUntil    *time.Time      `json:"heldUntil,omitempty"`
}

// CeilingStatus is one window's ceiling.
type CeilingStatus struct {
	WindowSeconds float64   `json:"windowSeconds"`
	Limit         float64   `json:"limit"`
	Source        string    `json:"source"`
	FoundAt       time.Time `json:"foundAt"`
}

// WindowCount is how many answered requests fell in one window.
type WindowCount struct {
	WindowSeconds float64 `json:"windowSeconds"`
	Count         int     `json:"count"`
}

// LearningEnabled reports whether the client learns limits at all.
func (c *Client) LearningEnabled() bool { return c.learn != nil }

// RateLimits lists every service with something learned, by name.
func (c *Client) RateLimits() []LimitStatus {
	out := []LimitStatus{}
	if c.learn == nil {
		return out
	}
	c.learn.mu.Lock()
	all := make([]*learner, 0, len(c.learn.all))
	for _, l := range c.learn.all {
		all = append(all, l)
	}
	c.learn.mu.Unlock()
	now := c.o.Now()
	for _, l := range all {
		if s, ok := l.status(now, c.learn.cfg); ok {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.Compare(out[i].Service, out[j].Service) < 0 })
	return out
}

func (l *learner) status(now time.Time, cfg learnCfg) (LimitStatus, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.ceil) == 0 && l.lastThrottle.IsZero() {
		return LimitStatus{}, false
	}
	s := LimitStatus{Service: l.key, Ceilings: []CeilingStatus{}, Probe: l.mode.String(), Probing: l.probe != nil,
		Pace: l.currentPace(cfg), LastThrottle: when(l.lastThrottle), CheckedAt: when(l.checkedAt),
		NextCheck: when(l.nextProbe(cfg))}
	if l.challenged {
		s.Probe = ProbeOff.String()
	}
	if l.hold.After(now) {
		s.HeldUntil = when(l.hold)
	}
	for _, c := range l.ceil {
		s.Ceilings = append(s.Ceilings, CeilingStatus{WindowSeconds: c.Window.Seconds(), Limit: c.Limit, Source: c.Source, FoundAt: c.At})
	}
	for i, w := range countWindows {
		s.Recent = append(s.Recent, WindowCount{WindowSeconds: w.Seconds(), Count: l.counts[i].count(now)})
	}
	return s, true
}

func when(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
