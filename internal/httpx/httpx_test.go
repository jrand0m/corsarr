package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
)

type sleeps struct {
	mu sync.Mutex
	d  []time.Duration
}

func (s *sleeps) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.d = append(s.d, d)
	s.mu.Unlock()
	return ctx.Err()
}

func client(t *testing.T, rep Reporter, sl *sleeps) *Client {
	t.Helper()
	return New(Options{Version: "1.2.3", Reporter: rep, Sleep: sl.sleep, Jitter: func(n int64) int64 { return n / 2 }})
}

// statusSeq serves the given statuses in order, then 200.
func statusSeq(codes []int, hdr map[string]string) (*httptest.Server, *atomic.Int32) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(n.Add(1)) - 1
		for k, v := range hdr {
			w.Header().Set(k, v)
		}
		w.Header().Set("X-UA", r.UserAgent())
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Body", string(body))
		if i < len(codes) {
			w.WriteHeader(codes[i])
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	return srv, &n
}

func TestRetriesIdempotentOn5xxWithJitteredBackoffAndUA(t *testing.T) {
	srv, n := statusSeq([]int{502, 503}, nil)
	defer srv.Close()
	sl := &sleeps{}
	resp, err := client(t, nil, sl).Do(mustReq(t, "GET", srv.URL, ""))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", resp, err)
	}
	defer resp.Body.Close()
	if n.Load() != 3 || resp.Header.Get("X-UA") != "corsarr/1.2.3" {
		t.Fatalf("attempts %d ua %q", n.Load(), resp.Header.Get("X-UA"))
	}
	if len(sl.d) != 2 || sl.d[0] != 250*time.Millisecond || sl.d[1] != 500*time.Millisecond {
		t.Fatalf("backoff: %v", sl.d)
	}
}

func TestPostIsNotRetriedOn5xxButIsOn429WithBody(t *testing.T) {
	srv, n := statusSeq([]int{500}, nil)
	defer srv.Close()
	resp, _ := client(t, nil, &sleeps{}).Do(mustReq(t, "POST", srv.URL, "x=1"))
	resp.Body.Close()
	if n.Load() != 1 || resp.StatusCode != 500 {
		t.Fatalf("POST retried: %d", n.Load())
	}
	srv2, n2 := statusSeq([]int{429}, map[string]string{"Retry-After": "2"})
	defer srv2.Close()
	sl := &sleeps{}
	resp, err := client(t, nil, sl).Do(mustReq(t, "POST", srv2.URL, "x=1"))
	if err != nil || resp.StatusCode != 200 || n2.Load() != 2 || resp.Header.Get("X-Body") != "x=1" {
		t.Fatalf("429 POST: %v %d body %q", err, n2.Load(), resp.Header.Get("X-Body"))
	}
	resp.Body.Close()
	if len(sl.d) != 1 || sl.d[0] != 2*time.Second {
		t.Fatalf("Retry-After not honoured: %v", sl.d)
	}
}

func TestLongRetryAfterIsReturnedAndReportedBackedOff(t *testing.T) {
	srv, n := statusSeq([]int{429, 429}, map[string]string{"Retry-After": "3600"})
	defer srv.Close()
	reg := health.New(nil, nil, nil)
	ref := domain.IntegrationRef{Type: "indexer", Key: "knaben"}
	resp, err := client(t, reg, &sleeps{}).For(ref).Do(mustReq(t, "GET", srv.URL, ""))
	if err != nil || resp.StatusCode != 429 || n.Load() != 1 {
		t.Fatalf("%v %d", err, n.Load())
	}
	resp.Body.Close()
	h, _ := reg.Get(ref)
	if h.Status != domain.HealthBackedOff || h.NextRetryAt == nil {
		t.Fatalf("health: %+v", h)
	}
}

func TestHealthClassesAndActionNeededStopsRetries(t *testing.T) {
	reg := health.New(nil, nil, nil)
	ref := domain.IntegrationRef{Type: "downloader", Key: "qbit"}
	srv, n := statusSeq([]int{401, 401, 401}, nil)
	defer srv.Close()
	c := client(t, reg, &sleeps{})
	resp, _ := c.For(ref).Do(mustReq(t, "GET", srv.URL, ""))
	resp.Body.Close()
	if h, _ := reg.Get(ref); h.Status != domain.HealthActionNeeded {
		t.Fatalf("401: %+v", h)
	}
	srv5, n5 := statusSeq([]int{500, 500, 500, 500, 500}, nil)
	defer srv5.Close()
	resp, _ = c.For(ref).Do(mustReq(t, "GET", srv5.URL, ""))
	resp.Body.Close()
	if n5.Load() != 1 {
		t.Fatalf("blocked integration retried %d times", n5.Load())
	}
	resp, _ = c.For(ref).Do(mustReq(t, "GET", srv.URL, ""))
	resp.Body.Close()
	if n.Load() != 2 {
		t.Fatal(n.Load())
	}
}

func TestClassify(t *testing.T) {
	now := time.Now()
	mk := func(code int, h ...string) *http.Response {
		r := &http.Response{StatusCode: code, Header: http.Header{}, Status: http.StatusText(code)}
		for i := 0; i+1 < len(h); i += 2 {
			r.Header.Set(h[i], h[i+1])
		}
		return r
	}
	cases := []struct {
		resp  *http.Response
		err   error
		class string
	}{
		{mk(200), nil, ""},
		{mk(404), nil, ""},
		{mk(403, "Cf-Mitigated", "challenge"), nil, health.ClassChallenge},
		{mk(403), nil, health.ClassAuth},
		{mk(503), nil, health.ClassServerError},
		{mk(429, "Retry-After", now.Add(time.Minute).UTC().Format(http.TimeFormat)), nil, health.ClassRateLimited},
		{nil, context.DeadlineExceeded, health.ClassTimeout},
		{nil, errors.New("connection refused"), health.ClassUnreachable},
	}
	for _, c := range cases {
		if o := Classify(c.resp, c.err, now); o.Class != c.class || o.OK != (c.class == "") {
			t.Errorf("%v %v: %+v", c.resp, c.err, o)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if d, ok := parseRetryAfter("7", now); !ok || d != 7*time.Second {
		t.Fatal(d)
	}
	if d, ok := parseRetryAfter(now.Add(-time.Hour).Format(http.TimeFormat), now); !ok || d != 0 {
		t.Fatal("past date")
	}
	for _, v := range []string{"", "soon"} {
		if _, ok := parseRetryAfter(v, now); ok {
			t.Fatal(v)
		}
	}
}

func TestPerHostTimeoutCoversBodyAndReportsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	reg := health.New(nil, nil, nil)
	ref := domain.IntegrationRef{Type: "indexer", Key: "slow"}
	c := New(Options{Reporter: reg, MaxRetries: -1})
	c.SetLimits("127.0.0.1", Limits{Timeout: 50 * time.Millisecond})
	_, err := c.For(ref).Do(mustReq(t, "GET", srv.URL, ""))
	if err == nil {
		t.Fatal("want timeout")
	}
	if h, _ := reg.Get(ref); h.LastErrorClass != health.ClassTimeout {
		t.Fatalf("%+v", h)
	}
}

func TestCancelledCallerIsNotReported(t *testing.T) {
	srv, _ := statusSeq([]int{503, 503, 503, 503}, nil)
	defer srv.Close()
	reg := health.New(nil, nil, nil)
	ref := domain.IntegrationRef{Type: "indexer", Key: "x"}
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	c := New(Options{Reporter: reg, Sleep: func(context.Context, time.Duration) error { cancel(); return context.Canceled }})
	if _, err := c.For(ref).Do(req); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, ok := reg.Get(ref); ok {
		t.Fatal("cancellation reported as failure")
	}
}

func TestNetworkErrorRetriedForGet(t *testing.T) {
	sl := &sleeps{}
	c := New(Options{Sleep: sl.sleep, Transport: failRT{}})
	if _, err := c.Do(mustReq(t, "GET", "http://x.invalid/", "")); err == nil || len(sl.d) != 3 {
		t.Fatalf("err %v sleeps %v", err, sl.d)
	}
	r, _ := http.NewRequest("POST", "http://x.invalid/", io.NopCloser(strings.NewReader("x"))) // not rewindable
	if _, err := c.Do(r); err == nil || len(sl.d) != 3 {
		t.Fatal("non-rewindable body retried")
	}
}

type failRT struct{}

func (failRT) RoundTrip(*http.Request) (*http.Response, error) { return nil, errors.New("refused") }

func TestRewindError(t *testing.T) {
	r, _ := http.NewRequest("PUT", "http://x/", strings.NewReader("a"))
	r.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("gone") }
	if _, err := rewind(r); err == nil {
		t.Fatal("want error")
	}
}

func TestBucket(t *testing.T) {
	t0 := time.Unix(0, 0)
	b := newBucket(2, 2, t0)
	if b.reserve(t0) != 0 || b.reserve(t0) != 0 {
		t.Fatal("burst")
	}
	if d := b.reserve(t0); d != 500*time.Millisecond {
		t.Fatalf("third: %v", d)
	}
	if d := b.reserve(t0); d != time.Second {
		t.Fatalf("fourth queues behind: %v", d)
	}
	if d := b.reserve(t0.Add(10 * time.Second)); d != 0 {
		t.Fatalf("refill: %v", d)
	}
}

func TestRateLimitWaitsThroughSleep(t *testing.T) {
	srv, _ := statusSeq(nil, nil)
	defer srv.Close()
	sl := &sleeps{}
	c := New(Options{Sleep: sl.sleep, Default: Limits{Rate: 1, Burst: 1}, Now: func() time.Time { return time.Unix(0, 0) }})
	for i := 0; i < 2; i++ {
		resp, err := c.Do(mustReq(t, "GET", srv.URL, ""))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if len(sl.d) != 1 || sl.d[0] != time.Second {
		t.Fatalf("waits: %v", sl.d)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepCtx(ctx, time.Hour) == nil || sleepCtx(context.Background(), time.Millisecond) != nil {
		t.Fatal("sleepCtx")
	}
}

func mustReq(t *testing.T, method, url, body string) *http.Request {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
