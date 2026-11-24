// Package musicbrainz is the built-in MusicBrainz music metadata plugin,
// implemented from the public MusicBrainz web service (version 2)
// documentation: artist search and lookup, release groups of an artist and
// the editions of a release group with their tracks. It talks to the web
// service directly, at most one request per second per host through one
// shared queue, with a User-Agent that names corsarr and a contact.
package musicbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// ID is the plugin id used as `type:` in config.
const ID = "musicbrainz"

const (
	defaultBase    = "https://musicbrainz.org/ws/2"
	officialHost   = "musicbrainz.org"
	defaultContact = "https://github.com/jrand0m/corsarr"
	maxJSON        = 8 << 20
	pageSize       = 100
	maxPages       = 30 // 3 000 release groups per artist at most
	maxRetryAfter  = 30 * time.Second
	busyBackoff    = 2 * time.Second
)

var manifest = plugin.Manifest{
	ID: ID, Kind: plugin.KindMetadata, Version: "1.0.0", APIVersion: plugin.APIVersion,
	Name: "MusicBrainz", Delivery: "builtin",
	Capabilities: []string{"music"},
	Settings: []plugin.Field{
		{Key: "baseUrl", Type: plugin.FieldURL, Label: "Web service address", Default: defaultBase,
			Help: "Change this only to use a MusicBrainz mirror; a mirror may allow a higher request rate."},
		{Key: "contact", Type: plugin.FieldText, Label: "Contact", Default: defaultContact,
			Help: "An address or URL sent with every request, as MusicBrainz asks of applications."},
		{Key: "requestsPerSecond", Type: plugin.FieldNumber, Label: "Requests per second", Default: 1, Min: ptr(0.1), Max: ptr(50),
			Help: "The public service allows one request per second; higher values only apply to a mirror."},
	},
}

func ptr(f float64) *float64 { return &f }

func init() {
	plugin.Default.MustRegister(plugin.Registration{Manifest: manifest, New: New})
}

// Client is one configured MusicBrainz instance.
type Client struct {
	inst     plugin.Instance
	base     string
	ua       string
	interval time.Duration
	queue    *pacer
}

// New is the plugin factory: settings are validated once, here.
func New(inst plugin.Instance) (any, error) {
	s := inst.Settings
	base := s.String("baseUrl")
	if base == "" {
		base = defaultBase
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "musicbrainz baseUrl %q must be an http(s) address", base)
	}
	contact := s.String("contact")
	if contact == "" {
		contact = defaultContact
	}
	return &Client{inst: inst, base: strings.TrimRight(base, "/"), ua: "( " + contact + " )",
		interval: Interval(u.Hostname(), rate(s)), queue: pacerFor(u.Host)}, nil
}

func rate(s plugin.Settings) float64 {
	switch v := s["requestsPerSecond"].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 1
}

// Interval is the spacing between requests: the configured rate, never
// faster than one request per second against the public service.
func Interval(host string, perSecond float64) time.Duration {
	if perSecond <= 0 || host == officialHost || strings.HasSuffix(host, "."+officialHost) {
		perSecond = min(max(perSecond, 0.1), 1)
	}
	return time.Duration(float64(time.Second) / perSecond)
}

// pacer is one request queue per host, shared by every instance, so two
// configured instances never double the rate.
type pacer struct {
	mu   sync.Mutex
	next time.Time
}

var (
	pacersMu sync.Mutex
	pacers   = map[string]*pacer{}
)

func pacerFor(host string) *pacer {
	pacersMu.Lock()
	defer pacersMu.Unlock()
	if p, ok := pacers[host]; ok {
		return p
	}
	p := &pacer{}
	pacers[host] = p
	return p
}

// wait reserves the next slot and sleeps until it.
func (p *pacer) wait(ctx context.Context, interval time.Duration) error {
	p.mu.Lock()
	now := time.Now()
	at := later(p.next, now)
	p.next = at.Add(interval)
	p.mu.Unlock()
	return sleep(ctx, at.Sub(now))
}

// hold pushes the queue back after the service said it is busy.
func (p *pacer) hold(d time.Duration) {
	p.mu.Lock()
	p.next = later(p.next, time.Now().Add(d))
	p.mu.Unlock()
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Test runs a one-result artist search.
func (c *Client) Test(ctx context.Context) error {
	var doc artistSearchDoc
	return c.getJSON(ctx, "/artist", url.Values{"query": {"test"}, "limit": {"1"}}, &doc)
}

// getJSON performs one paced GET; a 503 holds the whole queue back by the
// server's Retry-After and is retried once when the caller can wait that long.
func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	resp, err := c.do(ctx, path, q)
	if err == nil && resp.Status == http.StatusServiceUnavailable {
		wait := retryAfter(resp.Header)
		c.queue.hold(wait)
		if canWait(ctx, wait) {
			resp, err = c.do(ctx, path, q)
		}
	}
	if err != nil {
		return err
	}
	return decode(resp, path, out)
}

func (c *Client) do(ctx context.Context, path string, q url.Values) (*plugin.Response, error) {
	if err := c.queue.wait(ctx, c.interval); err != nil {
		return nil, plugin.Wrap(plugin.ClassifyTransport(err), "waiting for the request queue", err)
	}
	vals := url.Values{"fmt": {"json"}}
	for k, v := range q {
		vals[k] = v
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path+"?"+vals.Encode(), nil)
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassConfig, "building request", err)
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/json")
	return plugin.Do(c.inst.Env.HTTP(), req, maxJSON)
}

func decode(resp *plugin.Response, path string, out any) error {
	switch {
	case resp.Status == http.StatusNotFound:
		return fmt.Errorf("musicbrainz %s: %w", path, plugin.ErrNotFound)
	case plugin.ClassifyStatus(resp.Status) != "":
		return plugin.Errorf(plugin.ClassifyStatus(resp.Status), "musicbrainz answered HTTP %d", resp.Status)
	case resp.Status >= 400:
		return plugin.Errorf(plugin.ClassServerError, "musicbrainz answered HTTP %d", resp.Status)
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		return plugin.Wrap(plugin.ClassParse, "musicbrainz "+path, err)
	}
	return nil
}

func canWait(ctx context.Context, d time.Duration) bool {
	deadline, ok := ctx.Deadline()
	return !ok || time.Until(deadline) > d
}

func retryAfter(h http.Header) time.Duration {
	secs, err := strconv.Atoi(h.Get("Retry-After"))
	if err != nil || secs < 0 {
		return busyBackoff
	}
	return min(time.Duration(secs)*time.Second, maxRetryAfter)
}
