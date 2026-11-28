package plugin

import (
	"context"
	"net/http"

	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// DefaultMaxResponse caps a response body read through Doer when MaxBytes is
// zero; it equals the host's default message cap.
const DefaultMaxResponse = 8 << 20

// Doer sends requests through Do in the shape protocol clients commonly
// take: single-valued headers in and out, the whole body, and transport
// failures as err while any HTTP status comes back as status. Host failures
// (a refused host, a timeout, an unreachable server) are *abi.Error values,
// still reachable with errors.As after a client wraps them.
type Doer struct {
	// MaxBytes caps the response body; 0 means DefaultMaxResponse.
	MaxBytes int
}

// Do sends one request. A cancelled context sends nothing.
func (d Doer) Do(ctx context.Context, method, url string, header map[string]string, body []byte) (int, map[string]string, []byte, error) {
	if err := ctx.Err(); err != nil {
		return 0, nil, nil, err
	}
	req := Request{Method: method, URL: url, Headers: make(map[string][]string, len(header)), Body: body}
	for k, v := range header {
		req.Headers[http.CanonicalHeaderKey(k)] = []string{v}
	}
	resp, err := Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	limit := d.MaxBytes
	if limit <= 0 {
		limit = DefaultMaxResponse
	}
	if len(resp.Body) > limit {
		return 0, nil, nil, Errorf(abi.ClassParse, "response is larger than %d bytes", limit)
	}
	h := make(map[string]string, len(resp.Headers))
	for k, vs := range resp.Headers {
		if len(vs) > 0 {
			h[http.CanonicalHeaderKey(k)] = vs[0]
		}
	}
	return resp.Status, h, resp.Body, nil
}
