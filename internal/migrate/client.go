package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// Doer sends one HTTP request (the shared outbound client in production:
// timeouts, retries with backoff and per-host rate limits).
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// ReadOnly refuses every request that is not a GET, so no code path of a
// migration can change a source or a download client.
type ReadOnly struct{ Next Doer }

// Do forwards GET requests only.
func (r ReadOnly) Do(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return nil, fmt.Errorf("%w (%s %s)", ErrWriteRefused, req.Method, req.URL.Path)
	}
	return r.Next.Do(req)
}

// maxBody bounds one response; a library listing of a large source fits.
const maxBody = 64 << 20

// Client reads one source's API.
type Client struct {
	ep   Endpoint
	http Doer
}

// NewClient returns a read-only client for the endpoint.
func NewClient(ep Endpoint, d Doer) *Client { return &Client{ep: ep, http: ReadOnly{Next: d}} }

// get fetches path (below the app's API root) into out.
func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := *c.ep.URL
	u.Path += c.ep.App.apiBase() + path
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.ep.Key.Reveal())
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", c.ep.App, path, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorised
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("%s %s: %s", c.ep.App, path, resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return fmt.Errorf("%s %s: unreadable response: %w", c.ep.App, path, err)
	}
	return nil
}

// Status reads the app's name and version and checks the version range.
func (c *Client) Status(ctx context.Context) (wStatus, error) {
	var st wStatus
	if err := c.get(ctx, "/system/status", nil, &st); err != nil {
		return st, err
	}
	return st, CheckVersion(c.ep.App, st.Version)
}

// pageSize is the page size of paged resources.
const pageSize = 250

// paged reads a paged resource newest first until keep says stop or max
// records are read; memory stays bounded by max.
func paged[T any](ctx context.Context, c *Client, path string, extra url.Values, max int, keep func(T) bool) ([]T, error) {
	var out []T
	for page := 1; len(out) < max; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(pageSize)}}
		for k, v := range extra {
			q[k] = v
		}
		var p wPage[T]
		if err := c.get(ctx, path, q, &p); err != nil {
			return out, err
		}
		for _, r := range p.Records {
			if !keep(r) || len(out) >= max {
				return out, nil
			}
			out = append(out, r)
		}
		if len(p.Records) < pageSize || page*pageSize >= p.TotalRecords {
			return out, nil
		}
	}
	return out, nil
}
