package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// Request is one outbound HTTP request; Response its answer.
type (
	Request  = abi.HTTPRequest
	Response = abi.HTTPResponse
)

// Do sends a request through corsarr's shared client. The host refuses hosts
// outside permissions.http.hosts and outside the owner's egress allow list,
// does not follow redirects, and caps the response size.
func Do(req Request) (Response, error) {
	var r Response
	err := call(abi.HostHTTPDo, req, &r)
	return r, err
}

// Client is a small JSON-over-HTTP helper bound to a base URL.
type Client struct {
	Base    string
	Headers map[string][]string
}

// URL joins the base URL, a path and query parameters.
func (c Client) URL(path string, query url.Values) string {
	u := c.Base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

// JSON sends body (nil for none) as JSON and decodes a 2xx answer into out
// (nil discards it). Non-2xx answers become classified errors: 401/403 auth,
// 404 not_found, 429 rate_limited, 5xx server_error.
func (c Client) JSON(method, path string, query url.Values, body, out any) error {
	req := Request{Method: method, URL: c.URL(path, query), Headers: map[string][]string{"Accept": {"application/json"}}}
	for k, v := range c.Headers {
		req.Headers[k] = v
	}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		req.Body = b
		req.Headers["Content-Type"] = []string{"application/json"}
	}
	resp, err := Do(req)
	if err != nil {
		return err
	}
	if err := StatusError(resp.Status, resp.Body); err != nil {
		return err
	}
	if out == nil || len(resp.Body) == 0 {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(bytes.NewReader(resp.Body), int64(len(resp.Body)))).Decode(out); err != nil {
		return Errorf(abi.ClassParse, "unexpected answer: %v", err)
	}
	return nil
}

// StatusError classifies a non-2xx HTTP status (nil for 2xx).
func StatusError(status int, body []byte) error {
	if status >= 200 && status < 300 {
		return nil
	}
	msg := fmt.Sprintf("HTTP %d", status)
	if len(body) > 0 && len(body) < 200 {
		msg += ": " + string(body)
	}
	return Errorf(statusClass(status), "%s", msg)
}

// statusClass is the error class of a non-2xx status.
func statusClass(status int) string {
	switch {
	case status == 401 || status == 403:
		return abi.ClassAuth
	case status == 404:
		return abi.ClassNotFound
	case status == 429:
		return abi.ClassRateLimited
	case status >= 500:
		return abi.ClassServerError
	}
	return abi.ClassParse
}
