// Package client speaks the Transmission RPC protocol (JSON over HTTP) for
// Transmission 3.x and 4.x, implemented from the public RPC specification.
//
// The package has no dependencies beyond the standard library and never opens
// connections itself: every request goes through a Doer, so the same code runs
// on top of a plain HTTP client or inside a sandboxed module runtime.
package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Doer sends one HTTP request and returns the status, the response headers
// and the whole response body. Transport failures come back as err; any HTTP
// status, including errors, comes back as status with err nil. The Doer is
// responsible for bounding the response size.
type Doer interface {
	Do(ctx context.Context, method, url string, header map[string]string, body []byte) (status int, respHeader map[string]string, respBody []byte, err error)
}

// DefaultTimeout bounds one RPC request when Config.Timeout is zero.
const DefaultTimeout = 30 * time.Second

// rpcPath is where Transmission serves RPC when the address names no path.
const rpcPath = "/transmission/rpc"

const sessionHeader = "X-Transmission-Session-Id"

// Config is one configured Transmission instance.
type Config struct {
	// URL is the web interface or RPC address, for example
	// http://transmission:9091; without a path ending in /rpc the default
	// RPC path is appended.
	URL      string
	Username string
	Password string
	// CategoryDirs maps a category to the download directory as Transmission
	// sees it. Torrents of a category are added there; on servers without
	// labels the directory is also how a torrent's category is recognised.
	CategoryDirs map[string]string
	// PathMap maps folders as Transmission reports them (remote) onto the
	// same folders as the host sees them (local).
	PathMap map[string]string
	// AddPaused adds every torrent stopped, whatever the request asks.
	AddPaused bool
	// Timeout bounds one request; zero means DefaultTimeout.
	Timeout time.Duration
}

// Client is one Transmission instance. It is safe for concurrent use.
type Client struct {
	doer     Doer
	endpoint string
	auth     string
	paths    PathMap
	timeout  time.Duration
	addPause bool

	mu        sync.Mutex
	sessionID string
	info      *serverInfo
	dirs      map[string]string // category -> remote directory
	cats      map[string]bool   // categories configured, ensured or used
}

// serverInfo is what the client learns about the server once.
type serverInfo struct {
	Version    string
	RPCVersion int
}

// New validates the configuration and returns a client; no request is sent.
func New(doer Doer, cfg Config) (*Client, error) {
	endpoint, err := Endpoint(cfg.URL)
	if err != nil {
		return nil, err
	}
	paths, err := ParsePathMap(cfg.PathMap)
	if err != nil {
		return nil, wrap(ClassConfig, "path mapping", err)
	}
	c := &Client{doer: doer, endpoint: endpoint, paths: paths, timeout: cfg.Timeout, addPause: cfg.AddPaused,
		dirs: map[string]string{}, cats: map[string]bool{}}
	if c.timeout <= 0 {
		c.timeout = DefaultTimeout
	}
	if cfg.Username != "" || cfg.Password != "" {
		c.auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.Username+":"+cfg.Password))
	}
	for cat, dir := range cfg.CategoryDirs {
		if strings.TrimSpace(cat) == "" || strings.TrimSpace(dir) == "" {
			return nil, errorf(ClassConfig, "category directory %q=%q needs a category and a directory", cat, dir)
		}
		c.remember(cat, dir)
	}
	return c, nil
}

// Endpoint turns a configured address into the RPC endpoint URL.
func Endpoint(raw string) (string, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errorf(ClassConfig, "transmission url %q must be an http(s) address", raw)
	}
	if !strings.HasSuffix(u.Path, "/rpc") {
		u.Path += rpcPath
	}
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}

type rpcRequest struct {
	Method    string `json:"method"`
	Arguments any    `json:"arguments,omitempty"`
}

type rpcResponse struct {
	Result    string          `json:"result"`
	Arguments json.RawMessage `json:"arguments"`
}

// call runs one RPC method and decodes its arguments into out (nil skips).
func (c *Client) call(ctx context.Context, method string, args, out any) error {
	body, err := json.Marshal(rpcRequest{Method: method, Arguments: args})
	if err != nil {
		return wrap(ClassConfig, "encoding "+method, err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	status, resp, err := c.post(ctx, body)
	if err != nil {
		return err
	}
	return decode(method, status, resp, out)
}

// post sends the body in the current session. Transmission answers 409 with
// a fresh session id when the id is missing or stale: take it and retry once.
func (c *Client) post(ctx context.Context, body []byte) (int, []byte, error) {
	status, hdr, resp, err := c.send(ctx, body)
	if err == nil && status == 409 {
		c.setSessionID(header(hdr, sessionHeader))
		status, _, resp, err = c.send(ctx, body)
	}
	if err != nil {
		return 0, nil, wrap(transportClass(err), "transmission request failed", err)
	}
	return status, resp, nil
}

func (c *Client) send(ctx context.Context, body []byte) (int, map[string]string, []byte, error) {
	h := map[string]string{"Content-Type": "application/json"}
	if id := c.currentSessionID(); id != "" {
		h[sessionHeader] = id
	}
	if c.auth != "" {
		h["Authorization"] = c.auth
	}
	return c.doer.Do(ctx, "POST", c.endpoint, h, body)
}

func (c *Client) setSessionID(id string) {
	c.mu.Lock()
	c.sessionID = id
	c.mu.Unlock()
}

func (c *Client) currentSessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

// header reads a response header whatever its case.
func header(h map[string]string, name string) string {
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func decode(method string, status int, body []byte, out any) error {
	if err := statusError(status); err != nil {
		return err
	}
	var r rpcResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return wrap(ClassParse, "unexpected answer to "+method, err)
	}
	if r.Result != "success" {
		return resultError(method, r.Result)
	}
	if out == nil || len(r.Arguments) == 0 {
		return nil
	}
	if err := json.Unmarshal(r.Arguments, out); err != nil {
		return wrap(ClassParse, "unexpected arguments in answer to "+method, err)
	}
	return nil
}

func statusError(status int) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == 401:
		return errorf(ClassAuth, "transmission rejected the user name or password")
	case status == 403:
		return errorf(ClassAuth, "transmission refused this address; add it to the RPC whitelist or turn the whitelist off")
	case status == 409:
		return errorf(ClassServerError, "transmission kept refusing the session id")
	}
	return errorf(statusClass(status), "transmission answered HTTP %d", status)
}

// resultClasses classify the result strings that point at the payload rather
// than at the server.
var resultClasses = map[string]Class{
	"invalid or corrupt torrent file": ClassParse,
	"unrecognized info":               ClassParse,
}

func resultError(method, result string) error {
	if result == "" {
		return errorf(ClassParse, "transmission answered %s without a result", method)
	}
	class, ok := resultClasses[strings.ToLower(result)]
	if !ok {
		class = ClassServerError
	}
	return errorf(class, "transmission refused %s: %s", method, result)
}
