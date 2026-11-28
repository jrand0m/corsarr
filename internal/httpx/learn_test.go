package httpx

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
)

// fakeClock only moves when someone sleeps on it.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)} }

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func (c *fakeClock) sleep(ctx context.Context, d time.Duration) error {
	c.advance(d)
	return ctx.Err()
}

// windowServer enforces limit requests per sliding window and answers 429
// with Retry-After once it is used up; it can announce its limit instead.
type windowServer struct {
	mu        sync.Mutex
	clk       *fakeClock
	limit     int
	window    time.Duration
	advertise bool
	recent    []time.Time // accepted, inside the window
	sent      []time.Time // every accepted request
	throttles int
}

func (s *windowServer) setLimit(n int) {
	s.mu.Lock()
	s.limit = n
	s.mu.Unlock()
}

func (s *windowServer) RoundTrip(r *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clk.now()
	keep := s.recent[:0]
	for _, t := range s.recent {
		if t.After(now.Add(-s.window)) {
			keep = append(keep, t)
		}
	}
	s.recent = keep
	h := http.Header{}
	if s.advertise {
		h.Set("RateLimit-Limit", strconv.Itoa(s.limit))
		h.Set("RateLimit-Policy", strconv.Itoa(s.limit)+";w="+strconv.Itoa(int(s.window.Seconds())))
		h.Set("RateLimit-Remaining", strconv.Itoa(max(s.limit-len(s.recent)-1, 0)))
	}
	if len(s.recent) >= s.limit {
		s.throttles++
		wait := s.recent[0].Add(s.window).Sub(now)
		h.Set("Retry-After", strconv.Itoa(max(int(math.Ceil(wait.Seconds())), 1)))
		return answer(r, http.StatusTooManyRequests, h), nil
	}
	s.recent = append(s.recent, now)
	s.sent = append(s.sent, now)
	return answer(r, http.StatusOK, h), nil
}

// maxIn is the most accepted requests seen in any window of length w.
func (s *windowServer) maxIn(w time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	best, lo := 0, 0
	for hi := range s.sent {
		for !s.sent[lo].After(s.sent[hi].Add(-w)) {
			lo++
		}
		best = max(best, hi-lo+1)
	}
	return best
}

func (s *windowServer) throttled() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.throttles
}

func answer(r *http.Request, code int, h http.Header) *http.Response {
	return &http.Response{StatusCode: code, Status: http.StatusText(code), Header: h, Body: io.NopCloser(strings.NewReader("")), Request: r}
}

type memLimits struct {
	mu   sync.Mutex
	docs map[string][]byte
	fail error
}

func (m *memLimits) LoadRateLimits(context.Context) (map[string][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string][]byte{}
	for k, v := range m.docs {
		out[k] = v
	}
	return out, m.fail
}

func (m *memLimits) SaveRateLimit(_ context.Context, key string, doc []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.docs[key] = doc
	return nil
}

func learningClient(clk *fakeClock, rt http.RoundTripper, store LimitStore) *Client {
	return New(Options{Transport: rt, Now: clk.now, Sleep: clk.sleep, MaxRetries: -1,
		Default:  Limits{Rate: 1000, Burst: 1000},
		Learning: Learning{Enabled: true, Store: store}})
}

const svc = "http://svc.example.org/api"

func send(t *testing.T, d interface {
	Do(*http.Request) (*http.Response, error)
}, n int) {
	t.Helper()
	for range n {
		resp, err := d.Do(mustReq(t, "GET", svc, ""))
		if err != nil {
			continue
		}
		drain(resp)
	}
}

func status(t *testing.T, c *Client, service string) LimitStatus {
	t.Helper()
	for _, s := range c.RateLimits() {
		if s.Service == service {
			return s
		}
	}
	t.Fatalf("nothing learned for %s: %+v", service, c.RateLimits())
	return LimitStatus{}
}

func ceilingOf(s LimitStatus, window time.Duration) (CeilingStatus, bool) {
	for _, c := range s.Ceilings {
		if c.WindowSeconds == window.Seconds() {
			return c, true
		}
	}
	return CeilingStatus{}, false
}

// learned10 is a client that has met a 10-per-second service once.
func learned10(t *testing.T, store LimitStore) (*Client, *windowServer, *fakeClock) {
	t.Helper()
	clk := newFakeClock()
	srv := &windowServer{clk: clk, limit: 10, window: time.Second}
	c := learningClient(clk, srv, store)
	send(t, c, 300)
	return c, srv, clk
}

func TestLearnsFromThrottlesAndStaysBelow(t *testing.T) {
	c, srv, _ := learned10(t, nil)
	if srv.throttled() != 1 {
		t.Fatalf("throttled %d times, want only the first", srv.throttled())
	}
	s := status(t, c, "svc.example.org")
	ce, ok := ceilingOf(s, time.Second)
	if !ok || ce.Limit != 10 || ce.Source != SourceThrottle {
		t.Fatalf("ceiling %+v", s.Ceilings)
	}
	if s.Pace > 8.5 || s.Pace <= 0 || s.LastThrottle == nil || s.NextCheck == nil {
		t.Fatalf("status %+v", s)
	}
	if got := srv.maxIn(time.Second); got > 10 {
		t.Fatalf("server saw %d in a second", got)
	}
}

func TestEscalatesToLongerWindowWithoutRetryAfter(t *testing.T) {
	clk := newFakeClock()
	l := newLearner("x")
	cfg := learnCfg{headroom: 0.85, probeEvery: time.Hour}
	for range 10 {
		l.accepted(clk.now(), cfg)
	}
	l.throttled(clk.now(), 0, false, cfg)
	if c, ok := l.ceiling(time.Second); !ok || c.Limit != 10 {
		t.Fatalf("1s ceiling %+v", l.ceil)
	}
	clk.advance(5 * time.Second)
	for range 5 {
		l.accepted(clk.now(), cfg)
	}
	l.throttled(clk.now(), 0, false, cfg) // well under the 1 s ceiling: a longer window ran out
	if c, ok := l.ceiling(time.Minute); !ok || c.Limit != 15 {
		t.Fatalf("1m ceiling %+v", l.ceil)
	}
	if got := l.throttleWindow(clk.now(), time.Minute, true); got != time.Minute {
		t.Fatalf("next window %v", got)
	}
}

func TestUsesAdvertisedLimitAndNeverProbes(t *testing.T) {
	clk := newFakeClock()
	srv := &windowServer{clk: clk, limit: 20, window: 10 * time.Second, advertise: true}
	c := learningClient(clk, srv, nil)
	send(t, c, 200)
	if srv.throttled() != 0 {
		t.Fatalf("throttled %d times", srv.throttled())
	}
	s := status(t, c, "svc.example.org")
	ce, ok := ceilingOf(s, 10*time.Second)
	if !ok || ce.Limit != 20 || ce.Source != SourceAdvertised {
		t.Fatalf("ceiling %+v", s.Ceilings)
	}
	if s.NextCheck != nil {
		t.Fatalf("an advertised limit is never checked: %+v", s)
	}
	clk.advance(30 * 24 * time.Hour)
	send(t, c, 50)
	if status(t, c, "svc.example.org").Probing || srv.maxIn(10*time.Second) > 20 {
		t.Fatal("probed an advertised limit")
	}
}

// scripted answers one header set per request, then plain 200s.
type scripted struct {
	clk   *fakeClock
	hdrs  []map[string]string
	codes []int
	at    []time.Time
}

func (s *scripted) RoundTrip(r *http.Request) (*http.Response, error) {
	i := len(s.at)
	s.at = append(s.at, s.clk.now())
	h, code := http.Header{}, http.StatusOK
	if i < len(s.hdrs) {
		for k, v := range s.hdrs[i] {
			h.Set(k, v)
		}
	}
	if i < len(s.codes) {
		code = s.codes[i]
	}
	return answer(r, code, h), nil
}

func TestRemainingZeroHoldsUntilReset(t *testing.T) {
	clk := newFakeClock()
	rt := &scripted{clk: clk, hdrs: []map[string]string{{"X-RateLimit-Limit": "100", "X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "5"}}}
	c := learningClient(clk, rt, nil)
	send(t, c, 2)
	if gap := rt.at[1].Sub(rt.at[0]); gap < 5*time.Second {
		t.Fatalf("sent %v after remaining hit 0", gap)
	}
	if ce := status(t, c, "svc.example.org").Ceilings[0]; ce.WindowSeconds != 60 || ce.Limit != 100 {
		t.Fatalf("window guessed from the reset: %+v", ce)
	}
}

func TestHonoursRetryAfterExactly(t *testing.T) {
	clk := newFakeClock()
	rt := &scripted{clk: clk, codes: []int{429}, hdrs: []map[string]string{{"Retry-After": "7"}}}
	c := New(Options{Transport: rt, Now: clk.now, Sleep: clk.sleep, Learning: Learning{Enabled: true}})
	resp, err := c.Do(mustReq(t, "GET", svc, ""))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", resp, err)
	}
	drain(resp)
	if len(rt.at) != 2 || rt.at[1].Sub(rt.at[0]) != 7*time.Second {
		t.Fatalf("attempts at %v", rt.at)
	}
}

func TestLongRetryAfterFailsFastAsRateLimited(t *testing.T) {
	clk := newFakeClock()
	rt := &scripted{clk: clk, codes: []int{503}, hdrs: []map[string]string{{"Retry-After": "3600"}}}
	reg := health.New(nil, nil, nil)
	ref := domain.IntegrationRef{Type: "metadata", Key: "slow"}
	c := New(Options{Transport: rt, Now: clk.now, Sleep: clk.sleep, Reporter: reg, Learning: Learning{Enabled: true}})
	send(t, c.For(ref), 1)
	_, err := c.For(ref).Do(mustReq(t, "GET", svc, ""))
	var held ErrHeld
	if !errors.As(err, &held) || !held.Until.Equal(clk.now().Add(time.Hour)) || len(rt.at) != 1 {
		t.Fatalf("err %v, attempts %d", err, len(rt.at))
	}
	h, _ := reg.Get(ref)
	if h.LastErrorClass != health.ClassRateLimited || h.NextRetryAt == nil || !h.NextRetryAt.Equal(held.Until) {
		t.Fatalf("health %+v", h)
	}
	if s := status(t, c, RefKey(ref)); s.HeldUntil == nil || !strings.Contains(err.Error(), "wait until") {
		t.Fatalf("status %+v", s)
	}
}

func TestProbeFindsRaisedLimit(t *testing.T) {
	c, srv, clk := learned10(t, nil)
	srv.setLimit(12)
	clk.advance(8 * 24 * time.Hour)
	send(t, c, 1500)
	s := status(t, c, "svc.example.org")
	ce, _ := ceilingOf(s, time.Second)
	if ce.Source != SourceProbe || ce.Limit <= 10 || ce.Limit > 12 || s.Probing {
		t.Fatalf("after the check: %+v", s)
	}
	if srv.throttled() > 2 {
		t.Fatalf("throttled %d times; a check may cost one", srv.throttled())
	}
	before := srv.throttled()
	send(t, c, 300)
	if srv.throttled() != before {
		t.Fatal("throttled at the new pace")
	}
}

func TestProbeFindsLoweredLimit(t *testing.T) {
	c, srv, clk := learned10(t, nil)
	srv.setLimit(9)
	clk.advance(8 * 24 * time.Hour)
	send(t, c, 600)
	ce, _ := ceilingOf(status(t, c, "svc.example.org"), time.Second)
	if ce.Source != SourceProbe || ce.Limit >= 10 || ce.Limit < 8 {
		t.Fatalf("after the check: %+v", ce)
	}
}

func TestThrottleBelowCeilingLowersIt(t *testing.T) {
	c, srv, _ := learned10(t, nil)
	srv.setLimit(5)
	send(t, c, 300)
	ce, _ := ceilingOf(status(t, c, "svc.example.org"), time.Second)
	if ce.Limit > 7 || ce.Source != SourceThrottle {
		t.Fatalf("ceiling %+v", ce)
	}
	before := srv.throttled()
	send(t, c, 300)
	if srv.throttled() != before {
		t.Fatal("still throttled after relearning")
	}
}

func TestNoProbeWithoutQueuedDemand(t *testing.T) {
	c, srv, clk := learned10(t, nil)
	srv.setLimit(50)
	clk.advance(8 * 24 * time.Hour)
	for range 100 {
		send(t, c, 1)
		clk.advance(time.Second)
		if status(t, c, "svc.example.org").Probing {
			t.Fatal("a check started without queued requests")
		}
	}
	if ce, _ := ceilingOf(status(t, c, "svc.example.org"), time.Second); ce.Limit != 10 {
		t.Fatalf("ceiling moved without a check: %+v", ce)
	}
}

func TestHardCapsAreNeverExceeded(t *testing.T) {
	clk := newFakeClock()
	srv := &windowServer{clk: clk, limit: 10, window: time.Second}
	ref := domain.IntegrationRef{Type: "indexer", Key: "capped"}
	c := learningClient(clk, srv, nil)
	c.SetLimits("svc.example.org", Limits{Rate: 12, Burst: 1})
	c.Cap(ref, time.Second, 11)
	send(t, c.For(ref), 300)
	srv.setLimit(50)
	clk.advance(8 * 24 * time.Hour)
	send(t, c.For(ref), 1500)
	if got := srv.maxIn(time.Second); got > 13 {
		t.Fatalf("server saw %d in a second, above the configured rate", got)
	}
	ce, _ := ceilingOf(status(t, c, RefKey(ref)), time.Second)
	if ce.Limit > 11 {
		t.Fatalf("ceiling %v above the cap", ce.Limit)
	}
}

func TestProbeModesAndChallenges(t *testing.T) {
	cfg := learnCfg{headroom: 0.85, probeEvery: time.Hour}
	l := newLearner("x")
	l.setCeiling(Ceiling{Window: time.Second, Limit: 10, Source: SourceThrottle})
	if l.nextProbe(cfg).IsZero() {
		t.Fatal("an observed limit is checked")
	}
	l.mode = ProbeOff
	if !l.nextProbe(cfg).IsZero() || ProbeGentle.maxLevel() != 1.1 || ProbeGentle.String() != "gentle" {
		t.Fatal("probe off still scheduled")
	}
	clk := newFakeClock()
	rt := &scripted{clk: clk, codes: []int{403}, hdrs: []map[string]string{{"Cf-Mitigated": "challenge"}}}
	c := learningClient(clk, rt, nil)
	ref := domain.IntegrationRef{Type: "indexer", Key: "scraped"}
	c.SetProbe(ref, ProbeGentle)
	send(t, c.For(ref), 1)
	c.Throttled(ref, 0)
	if s := status(t, c, RefKey(ref)); s.Probe != "off" || s.NextCheck != nil {
		t.Fatalf("challenge left probing on: %+v", s)
	}
}

func TestPluginThrottleAndDeclaredLimit(t *testing.T) {
	clk := newFakeClock()
	c := learningClient(clk, &scripted{clk: clk}, nil)
	ref := domain.IntegrationRef{Type: "indexer", Key: "api"}
	send(t, c.For(ref), 5)
	c.Throttled(ref, time.Second)
	ce, _ := ceilingOf(status(t, c, RefKey(ref)), time.Second)
	if ce.Limit != 5 || ce.Source != SourceThrottle {
		t.Fatalf("plugin throttle: %+v", ce)
	}
	other := domain.IntegrationRef{Type: "metadata", Key: "doc"}
	c.Declare(other, time.Minute, 40)
	c.Declare(other, time.Minute, 40)
	s := status(t, c, RefKey(other))
	if len(s.Ceilings) != 1 || s.Ceilings[0].Source != SourceDeclared || s.NextCheck != nil {
		t.Fatalf("declared: %+v", s)
	}
}

func TestPersistsAcrossRestart(t *testing.T) {
	store := &memLimits{docs: map[string][]byte{"broken": []byte("{")}}
	_, _, clk := learned10(t, store)
	srv := &windowServer{clk: clk, limit: 10, window: time.Second}
	c := learningClient(clk, srv, store)
	if err := c.RestoreLimits(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := status(t, c, "svc.example.org")
	if ce, ok := ceilingOf(s, time.Second); !ok || ce.Limit != 10 || s.CheckedAt == nil {
		t.Fatalf("restored %+v", s)
	}
	send(t, c, 200)
	if srv.throttled() != 0 {
		t.Fatalf("relearned from zero: %d throttles", srv.throttled())
	}
	store.fail = errors.New("disk")
	if c.RestoreLimits(context.Background()) == nil {
		t.Fatal("load error swallowed")
	}
}

func TestLearningOff(t *testing.T) {
	c := New(Options{})
	ref := domain.IntegrationRef{Type: "indexer", Key: "a"}
	c.Throttled(ref, time.Second)
	c.SetProbe(ref, ProbeOff)
	if c.LearningEnabled() || len(c.RateLimits()) != 0 || c.RestoreLimits(context.Background()) != nil {
		t.Fatal("learning should be off")
	}
	if c := New(Options{Learning: Learning{Enabled: true, Headroom: 2}}); c.learn.cfg.headroom != 0.85 || !c.LearningEnabled() {
		t.Fatal("headroom default")
	}
}

func TestParseAdvert(t *testing.T) {
	now := newFakeClock().now()
	cases := []struct {
		h      map[string]string
		limit  float64
		window time.Duration
	}{
		{map[string]string{"RateLimit-Limit": "100, 100;w=60"}, 100, time.Minute},
		{map[string]string{"RateLimit-Policy": `"default";q=50;w=10`}, 50, 10 * time.Second},
		{map[string]string{"X-RateLimit-Limit": "5000", "X-RateLimit-Reset": strconv.FormatInt(now.Add(30*time.Minute).Unix(), 10)}, 5000, time.Hour},
		{map[string]string{"X-RateLimit-Limit": "60"}, 60, time.Hour},
		{map[string]string{"X-RateLimit-Limit": "junk"}, 0, time.Hour},
	}
	for _, tc := range cases {
		h := http.Header{}
		for k, v := range tc.h {
			h.Set(k, v)
		}
		a := parseAdvert(h, now)
		if a.limit != tc.limit || a.window != tc.window {
			t.Errorf("%v: got %v per %v", tc.h, a.limit, a.window)
		}
	}
	if windowFor(2*time.Hour) != time.Hour || number("-1") != 0 {
		t.Fatal("helpers")
	}
}

func TestSlidingCountForgetsOldSlots(t *testing.T) {
	clk := newFakeClock()
	s := newSliding(time.Second)
	for range 5 {
		s.add(clk.now())
	}
	clk.advance(500 * time.Millisecond)
	s.add(clk.now())
	if s.count(clk.now()) != 6 {
		t.Fatal("count inside window")
	}
	clk.advance(700 * time.Millisecond)
	if n := s.count(clk.now()); n != 1 {
		t.Fatalf("count after window %d", n)
	}
	clk.advance(time.Hour)
	if s.count(clk.now()) != 0 || s.count(clk.now().Add(-time.Minute)) != 0 {
		t.Fatal("count after a long gap")
	}
}

func TestAdvertisedReplacesObservedAndKeepsLongestGuess(t *testing.T) {
	now := newFakeClock().now()
	l := newLearner("x")
	l.setCeiling(Ceiling{Window: time.Second, Limit: 3, Source: SourceThrottle})
	l.setCeiling(Ceiling{Window: time.Hour, Limit: 900, Source: SourceDeclared})
	l.probe = &probe{window: time.Second}
	if !l.advertised(advert{limit: 100, window: time.Minute, guessed: true, remaining: -1}, now) {
		t.Fatal("first announcement")
	}
	if l.advertised(advert{limit: 100, window: time.Second, guessed: true, remaining: -1}, now) {
		t.Fatal("a shorter guess must not shrink the window")
	}
	if _, ok := l.ceiling(time.Second); ok || l.probe != nil || len(l.ceil) != 2 {
		t.Fatalf("observed ceilings kept: %+v", l.ceil)
	}
	if l.count(42*time.Second, now) != 0 {
		t.Fatal("unknown window counts nothing")
	}
}
