// Package tmdb is the built-in TMDB movie and TV metadata plugin, implemented from the
// public TMDB API v3 documentation.
package tmdb

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
const ID = "tmdb"

const (
	defaultBase   = "https://api.themoviedb.org/3"
	defaultImages = "https://image.tmdb.org/t/p/"
	posterSize    = "w500"
	backdropSize  = "w1280"
	maxJSON       = 4 << 20
	minInterval   = 50 * time.Millisecond // 20 requests per second, well under TMDB's limit
	maxRetryAfter = 10 * time.Second
)

var manifest = plugin.Manifest{
	ID: ID, Kind: plugin.KindMetadata, Version: "1.0.0", APIVersion: plugin.APIVersion,
	Name: "TMDB", Delivery: "builtin",
	Capabilities: []string{"movie", "tv"},
	Settings: []plugin.Field{
		{Key: "apiKey", Type: plugin.FieldSecret, Label: "API key or read access token", Required: true,
			Help: "TMDB v3 API key, or the v4 read access token (a long value starting with eyJ). Both come from your TMDB account's API settings."},
		{Key: "language", Type: plugin.FieldText, Label: "Language", Default: "en-US",
			Help: "Language of titles and overviews as an IETF tag, for example en-US or de-DE."},
		{Key: "region", Type: plugin.FieldText, Label: "Release region",
			Help: "Two-letter country code (for example US) whose cinema, digital and physical release dates are used. Empty uses the earliest date worldwide."},
		{Key: "baseUrl", Type: plugin.FieldURL, Label: "API address", Default: defaultBase,
			Help: "Only change this to reach TMDB through a mirror or proxy."},
	},
}

func init() {
	plugin.Default.MustRegister(plugin.Registration{Manifest: manifest, New: New})
}

// Client is one configured TMDB instance.
type Client struct {
	inst     plugin.Instance
	base     string
	key      string
	language string
	region   string

	mu     sync.Mutex
	last   time.Time // time of the previous request, for pacing
	images string    // poster base URL from /configuration
}

// New is the plugin factory: settings are validated once, here.
func New(inst plugin.Instance) (any, error) {
	s := inst.Settings
	key := s.String("apiKey")
	if key == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "tmdb needs an apiKey")
	}
	base := s.String("baseUrl")
	if base == "" {
		base = defaultBase
	}
	if u, err := url.Parse(base); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "tmdb baseUrl %q must be an http(s) address", base)
	}
	lang := s.String("language")
	if lang == "" {
		lang = "en-US"
	}
	return &Client{inst: inst, base: strings.TrimRight(base, "/"), key: key, language: lang,
		region: strings.ToUpper(s.String("region"))}, nil
}

// bearer reports whether the key is a v4 read access token (a JWT).
func (c *Client) bearer() bool { return strings.HasPrefix(c.key, "eyJ") }

// Test reads the API configuration, which needs a valid key.
func (c *Client) Test(ctx context.Context) error {
	_, err := c.imageBase(ctx)
	return err
}

type configuration struct {
	Images struct {
		SecureBaseURL string `json:"secure_base_url"`
	} `json:"images"`
}

func (c *Client) imageBase(ctx context.Context) (string, error) {
	c.mu.Lock()
	cached := c.images
	c.mu.Unlock()
	if cached != "" {
		return cached, nil
	}
	var cfg configuration
	if err := c.getJSON(ctx, "/configuration", nil, &cfg); err != nil {
		return "", err
	}
	base := cfg.Images.SecureBaseURL
	if base == "" {
		base = defaultImages
	}
	c.mu.Lock()
	c.images = base
	c.mu.Unlock()
	return base, nil
}

// getJSON performs a paced GET and decodes the JSON answer. One 429 is
// retried after the server's Retry-After (capped).
func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	resp, err := c.do(ctx, path, q)
	if err == nil && resp.Status == http.StatusTooManyRequests {
		if sleep(ctx, retryAfter(resp.Header)) != nil {
			return plugin.Errorf(plugin.ClassRateLimited, "tmdb rate limit not lifted in time")
		}
		resp, err = c.do(ctx, path, q)
	}
	if err != nil {
		return err
	}
	return decode(resp, path, out)
}

func decode(resp *plugin.Response, path string, out any) error {
	switch {
	case resp.Status == http.StatusNotFound:
		return fmt.Errorf("tmdb %s: %w", path, plugin.ErrNotFound)
	case plugin.ClassifyStatus(resp.Status) != "":
		return plugin.Errorf(plugin.ClassifyStatus(resp.Status), "tmdb answered HTTP %d", resp.Status)
	case resp.Status >= 400:
		return plugin.Errorf(plugin.ClassServerError, "tmdb answered HTTP %d", resp.Status)
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		return plugin.Wrap(plugin.ClassParse, "tmdb "+path, err)
	}
	return nil
}

func (c *Client) do(ctx context.Context, path string, q url.Values) (*plugin.Response, error) {
	if err := c.pace(ctx); err != nil {
		return nil, plugin.Wrap(plugin.ClassifyTransport(err), "waiting for rate limit", err)
	}
	vals := url.Values{"language": {c.language}}
	for k, v := range q {
		vals[k] = v
	}
	if !c.bearer() {
		vals.Set("api_key", c.key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path+"?"+vals.Encode(), nil)
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassConfig, "building request", err)
	}
	if c.bearer() {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	return plugin.Do(c.inst.Env.HTTP(), req, maxJSON)
}

// pace keeps requests at least minInterval apart.
func (c *Client) pace(ctx context.Context) error {
	c.mu.Lock()
	wait := minInterval - time.Since(c.last)
	if wait < 0 {
		wait = 0
	}
	c.last = time.Now().Add(wait)
	c.mu.Unlock()
	return sleep(ctx, wait)
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

func retryAfter(h http.Header) time.Duration {
	secs, err := strconv.Atoi(h.Get("Retry-After"))
	if err != nil || secs < 0 {
		return time.Second
	}
	return min(time.Duration(secs)*time.Second, maxRetryAfter)
}
