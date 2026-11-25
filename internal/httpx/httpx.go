// Package httpx is the one shared outbound HTTP client: connection reuse, per-host timeout and token-bucket rate
// limit, jittered bounded retries for idempotent requests and 429/5xx
// (Retry-After honoured), User-Agent corsarr/<version>, and health
// reporting per integration. It implements plugin.HTTPDoer.
package httpx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// Reporter receives per-integration results (health.Registry implements it).
type Reporter interface {
	Report(ref domain.IntegrationRef, o health.Observation) domain.IntegrationHealth
}

// Limits are the per-host knobs.
type Limits struct {
	Timeout time.Duration // per attempt, including reading the body
	Rate    float64       // requests per second
	Burst   int
}

// Options configure a Client; zero values take the defaults.
type Options struct {
	Version     string
	Default     Limits
	MaxRetries  int // 0 = default 3; negative = no retries
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	Transport   http.RoundTripper
	Reporter    Reporter
	Sleep       func(context.Context, time.Duration) error
	Now         func() time.Time
	Jitter      func(n int64) int64 // uniform [0,n)
}

// Client is safe for concurrent use.
type Client struct {
	o     Options
	mu    sync.Mutex
	hosts map[string]*host
}

type host struct {
	limits Limits
	bucket *bucket
}

var _ plugin.HTTPDoer = (*Client)(nil)

// New returns a client with defaults: 30 s timeout, 5 req/s burst 10 per host,
// 3 retries, 500 ms base / 30 s max backoff.
func New(o Options) *Client {
	o.Default = withDefaults(o.Default, Limits{Timeout: 30 * time.Second, Rate: 5, Burst: 10})
	o.MaxRetries = pick(o.MaxRetries, 3)
	o.BaseBackoff = pick(o.BaseBackoff, 500*time.Millisecond)
	o.MaxBackoff = pick(o.MaxBackoff, 30*time.Second)
	if o.Transport == nil {
		o.Transport = &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: 16, MaxIdleConnsPerHost: 4,
			IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ForceAttemptHTTP2: true}
	}
	if o.Sleep == nil {
		o.Sleep = sleepCtx
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Jitter == nil {
		o.Jitter = func(n int64) int64 { return rand.Int64N(max(n, 1)) }
	}
	return &Client{o: o, hosts: map[string]*host{}}
}

func pick[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}

func withDefaults(l, def Limits) Limits {
	return Limits{Timeout: pick(l.Timeout, def.Timeout), Rate: pick(l.Rate, def.Rate), Burst: pick(l.Burst, def.Burst)}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SetLimits overrides one host's limits (e.g. an indexer's documented rate).
func (c *Client) SetLimits(hostname string, l Limits) {
	l = withDefaults(l, c.o.Default)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hosts[hostname] = &host{limits: l, bucket: newBucket(l.Rate, l.Burst, c.o.Now())}
}

func (c *Client) host(name string) *host {
	c.mu.Lock()
	defer c.mu.Unlock()
	h, ok := c.hosts[name]
	if !ok {
		l := c.o.Default
		h = &host{limits: l, bucket: newBucket(l.Rate, l.Burst, c.o.Now())}
		c.hosts[name] = h
	}
	return h
}

// Do sends a request without health attribution.
func (c *Client) Do(req *http.Request) (*http.Response, error) { return c.do(req, nil) }

// For returns a doer that reports every final result to the health entry of
// ref; this is what plugin instances receive as Env.HTTP().
func (c *Client) For(ref domain.IntegrationRef) plugin.HTTPDoer { return doerFor{c: c, ref: &ref} }

type doerFor struct {
	c   *Client
	ref *domain.IntegrationRef
}

func (d doerFor) Do(req *http.Request) (*http.Response, error) { return d.c.do(req, d.ref) }

func (c *Client) do(req *http.Request, ref *domain.IntegrationRef) (*http.Response, error) {
	req.Header.Set("User-Agent", userAgent(req.Header.Get("User-Agent"), pick(c.o.Version, "dev")))
	h := c.host(req.URL.Hostname())
	retries := c.o.MaxRetries
	if ref != nil && c.blocked(*ref) {
		retries = 0 // action needed: one attempt so a fixed credential recovers, no blind retries
	}
	resp, err := c.attempts(req, h, retries)
	c.report(ref, resp, err)
	return resp, err
}

// userAgent is corsarr's product token; a plugin may add a comment such as a
// contact address ("( https://… )"), which follows the token, while a full
// User-Agent a plugin sets stays as it is.
func userAgent(set, version string) string {
	product := "corsarr/" + version
	switch {
	case set == "":
		return product
	case strings.HasPrefix(set, "("):
		return product + " " + set
	}
	return set
}

func (c *Client) blocked(ref domain.IntegrationRef) bool {
	b, ok := c.o.Reporter.(interface {
		Blocked(domain.IntegrationRef) bool
	})
	return ok && b.Blocked(ref)
}

func (c *Client) attempts(req *http.Request, h *host, retries int) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := c.once(req, h)
		wait, retry := c.retryAfter(req, resp, err, attempt, retries)
		if !retry {
			return resp, err
		}
		drain(resp)
		if err := c.o.Sleep(req.Context(), wait); err != nil {
			return nil, err
		}
		if req, err = rewind(req); err != nil {
			return nil, err
		}
	}
}

// once waits for a token and sends one attempt bounded by the host timeout.
func (c *Client) once(req *http.Request, h *host) (*http.Response, error) {
	if d := h.bucket.reserve(c.o.Now()); d > 0 {
		if err := c.o.Sleep(req.Context(), d); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(req.Context(), h.limits.Timeout)
	resp, err := c.o.Transport.RoundTrip(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// cancelBody releases the attempt's timeout when the caller closes the body.
type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error { err := b.ReadCloser.Close(); b.cancel(); return err }

// retryAfter decides whether to retry and how long to wait (pure apart
// from jitter and clock).
func (c *Client) retryAfter(req *http.Request, resp *http.Response, err error, attempt, retries int) (time.Duration, bool) {
	if attempt >= retries || req.Context().Err() != nil || !retryable(req, resp, err) || challenged(resp) {
		return 0, false // an anti-bot challenge answers the same way until someone acts
	}
	if resp != nil {
		if d, ok := parseRetryAfter(resp.Header.Get("Retry-After"), c.o.Now()); ok {
			return d, d <= c.o.MaxBackoff // a longer wait is the caller's (and health's) business
		}
	}
	ceil := min(c.o.BaseBackoff<<attempt, c.o.MaxBackoff)
	return time.Duration(c.o.Jitter(int64(ceil))), true
}

var idempotent = map[string]bool{http.MethodGet: true, http.MethodHead: true, http.MethodOptions: true, http.MethodPut: true, http.MethodDelete: true}

func retryable(req *http.Request, resp *http.Response, err error) bool {
	rewindable := req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
	if !rewindable {
		return false
	}
	if err != nil {
		return idempotent[req.Method]
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return true // the server refused the request, so even a POST is safe to resend
	}
	return resp.StatusCode >= 500 && idempotent[req.Method]
}

func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if s, err := strconv.Atoi(v); err == nil {
		return time.Duration(max(s, 0)) * time.Second, true
	}
	t, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}
	return max(t.Sub(now), 0), true
}

func rewind(req *http.Request) (*http.Request, error) {
	if req.GetBody == nil {
		return req, nil
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	r := req.Clone(req.Context())
	r.Body = body
	return r, nil
}

func drain(resp *http.Response) {
	if resp != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
	}
}

// challenged reports whether resp is an anti-bot page. Only 403/503 bodies
// are looked at; the part read is put back so the caller still sees it all.
func challenged(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	var head []byte
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusServiceUnavailable {
		head = peek(resp, plugin.ChallengeScan)
	}
	return plugin.Challenged(resp.StatusCode, resp.Header, head)
}

// peek reads up to n bytes of the body and restores them in front of the rest.
func peek(resp *http.Response, n int64) []byte {
	if resp.Body == nil {
		return nil
	}
	head, _ := io.ReadAll(io.LimitReader(resp.Body, n))
	resp.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(head), resp.Body), resp.Body}
	return head
}

// Classify maps a final result to a health observation. A 403/503 body is
// inspected for an anti-bot page (and left readable).
func Classify(resp *http.Response, err error, now time.Time) health.Observation {
	if err != nil {
		return health.Observation{Class: errClass(err), Message: err.Error()}
	}
	switch code := resp.StatusCode; {
	case challenged(resp):
		return health.Observation{Class: health.ClassChallenge, Message: plugin.ChallengeError(code, resp.Header).Msg}
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return health.Observation{Class: health.ClassAuth, Message: resp.Status}
	case code == http.StatusTooManyRequests:
		o := health.Observation{Class: health.ClassRateLimited, Message: resp.Status}
		if d, ok := parseRetryAfter(resp.Header.Get("Retry-After"), now); ok {
			at := now.Add(d)
			o.NextRetry = &at
		}
		return o
	case code >= 500:
		return health.Observation{Class: health.ClassServerError, Message: resp.Status}
	}
	return health.Observation{OK: true}
}

func errClass(err error) string {
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return health.ClassTimeout
	}
	return health.ClassUnreachable
}

func (c *Client) report(ref *domain.IntegrationRef, resp *http.Response, err error) {
	if ref == nil || c.o.Reporter == nil || errors.Is(err, context.Canceled) {
		return // a caller cancelling is not the integration's fault
	}
	c.o.Reporter.Report(*ref, Classify(resp, err, c.o.Now()))
}
