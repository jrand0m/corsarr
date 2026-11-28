// Package client speaks rTorrent's XML-RPC interface over HTTP: the /RPC2
// endpoint a web front end or reverse proxy exposes, optionally behind basic
// auth. It has no dependencies beyond the standard library and no direct
// network access: every request goes through a Doer, so the package runs in
// a sandbox that only offers an HTTP helper.
package client

import (
	"context"
	"encoding/base64"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Doer performs one HTTP request and returns the status, the response
// headers and the response body.
type Doer interface {
	Do(ctx context.Context, method, url string, header map[string]string, body []byte) (status int, respHeader map[string]string, respBody []byte, err error)
}

// Config is the connection to one rTorrent instance.
type Config struct {
	// URL is the XML-RPC endpoint; a URL without a path gets /RPC2.
	URL      string
	Username string
	Password string
	// LabelField is the custom field holding the label (category):
	// custom1 (default) to custom4. custom5 is left to the data-erasing
	// helper of web front ends.
	LabelField string
	// PathMap maps rTorrent's paths onto the caller's view.
	PathMap PathMap
	// Timeout bounds each call; 0 means DefaultTimeout.
	Timeout time.Duration
	// MaxResponse caps one response body; 0 means DefaultMaxResponse.
	MaxResponse int
}

// DefaultTimeout bounds one call when the configuration names none.
const DefaultTimeout = 30 * time.Second

// Client talks to one rTorrent instance. It is safe for concurrent use.
type Client struct {
	doer     Doer
	cfg      Config
	endpoint string
	auth     string

	mu       sync.Mutex
	missing  map[string]bool // methods the server reported unknown
	eraseKey *bool           // whether a data-erasing handler is installed
}

var labelFields = map[string]bool{"custom1": true, "custom2": true, "custom3": true, "custom4": true}

// New checks the configuration and returns a client.
func New(doer Doer, cfg Config) (*Client, error) {
	if doer == nil {
		return nil, errorf(KindConfig, "no HTTP client")
	}
	endpoint, err := endpointOf(cfg.URL)
	if err != nil {
		return nil, err
	}
	if cfg.LabelField == "" {
		cfg.LabelField = "custom1"
	}
	if !labelFields[cfg.LabelField] {
		return nil, errorf(KindConfig, "label field must be custom1 to custom4, not %q", cfg.LabelField)
	}
	c := &Client{doer: doer, cfg: cfg, endpoint: endpoint, missing: map[string]bool{}}
	if cfg.Username != "" || cfg.Password != "" {
		c.auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.Username+":"+cfg.Password))
	}
	return c, nil
}

// endpointOf validates the URL and adds /RPC2 when it has no path.
func endpointOf(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errorf(KindConfig, "url must be an http(s) address of the XML-RPC endpoint")
	}
	if u.User != nil {
		return "", errorf(KindConfig, "put credentials in username and password, not in the url")
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/RPC2"
	}
	return u.String(), nil
}

// Endpoint is the XML-RPC URL requests go to.
func (c *Client) Endpoint() string { return c.endpoint }

// Call invokes one XML-RPC method and returns its decoded value.
func (c *Client) Call(ctx context.Context, method string, params ...any) (any, error) {
	body, err := EncodeCall(method, params...)
	if err != nil {
		return nil, err
	}
	resp, err := c.post(ctx, body)
	if err != nil {
		return nil, err
	}
	v, err := DecodeResponse(resp, c.cfg.MaxResponse)
	if f, ok := err.(*Fault); ok {
		return nil, c.faultError(method, f)
	}
	return v, err
}

// faultError classifies a fault and remembers methods the server lacks.
func (c *Client) faultError(method string, f *Fault) error {
	fe := classifyFault(method, f)
	if fe.Kind == KindUnsupported {
		c.markMissing(method)
	}
	return fe
}

func (c *Client) post(ctx context.Context, body []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	header := map[string]string{"Content-Type": "text/xml"}
	if c.auth != "" {
		header["Authorization"] = c.auth
	}
	status, _, resp, err := c.doer.Do(ctx, "POST", c.endpoint, header, body)
	if err != nil {
		return nil, wrap(classifyTransport(err), "XML-RPC request", err)
	}
	return resp, statusError(status)
}

func (c *Client) timeout() time.Duration {
	if c.cfg.Timeout > 0 {
		return c.cfg.Timeout
	}
	return DefaultTimeout
}

func statusError(status int) error {
	switch classifyStatus(status) {
	case "":
		return nil
	case KindAuth:
		return errorf(KindAuth, "rTorrent endpoint refused the credentials (HTTP %d)", status)
	case KindConfig:
		return errorf(KindConfig, "no XML-RPC endpoint at this url (HTTP %d)", status)
	}
	return errorf(KindServer, "rTorrent endpoint answered HTTP %d", status)
}

func (c *Client) markMissing(method string) {
	c.mu.Lock()
	c.missing[method] = true
	c.mu.Unlock()
}

func (c *Client) isMissing(method string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.missing[method]
}

// callFirst invokes the first of the alternative method names the server
// knows; names reported unknown once are skipped afterwards.
func (c *Client) callFirst(ctx context.Context, names []string, params ...any) (any, error) {
	var last error = errorf(KindUnsupported, "%s", names[0])
	for _, name := range names {
		if c.isMissing(name) {
			continue
		}
		v, err := c.Call(ctx, name, params...)
		if KindOf(err) != KindUnsupported {
			return v, err
		}
		last = err
	}
	return nil, last
}

// Request is one call of a batch.
type Request struct {
	Method string
	Params []any
}

// Batch runs the requests in one system.multicall round trip and returns
// their values in order; the first failed request ends it with that
// request's error. A server without system.multicall gets the requests one
// by one.
func (c *Client) Batch(ctx context.Context, reqs []Request) ([]any, error) {
	if c.isMissing("system.multicall") {
		return c.sequential(ctx, reqs)
	}
	calls := make([]any, len(reqs))
	for i, r := range reqs {
		params := r.Params
		if params == nil {
			params = []any{}
		}
		calls[i] = map[string]any{"methodName": r.Method, "params": params}
	}
	v, err := c.Call(ctx, "system.multicall", calls)
	if KindOf(err) == KindUnsupported {
		return c.sequential(ctx, reqs)
	}
	if err != nil {
		return nil, err
	}
	return c.unpackBatch(reqs, v)
}

func (c *Client) unpackBatch(reqs []Request, v any) ([]any, error) {
	items, ok := v.([]any)
	if !ok || len(items) != len(reqs) {
		return nil, malformed("system.multicall answered %d results for %d calls", len(items), len(reqs))
	}
	out := make([]any, len(items))
	for i, it := range items {
		if one, ok := it.([]any); ok && len(one) == 1 {
			out[i] = one[0]
			continue
		}
		m, _ := it.(map[string]any)
		code, _ := m["faultCode"].(int64)
		msg, _ := m["faultString"].(string)
		return nil, c.faultError(reqs[i].Method, &Fault{Code: int(code), Message: msg})
	}
	return out, nil
}

func (c *Client) sequential(ctx context.Context, reqs []Request) ([]any, error) {
	out := make([]any, len(reqs))
	for i, r := range reqs {
		v, err := c.Call(ctx, r.Method, r.Params...)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
