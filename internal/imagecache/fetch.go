package imagecache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Fetch limits.
const (
	maxSource    = 20 << 20
	maxRedirects = 3
)

// ErrMissing is a provider answer that the image does not exist.
var ErrMissing = errors.New("the provider has no such image")

// Doer sends one request (the shared outbound client).
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// fetch downloads an origin address, following at most maxRedirects
// redirects; each hop goes through the shared client, so the egress
// allow-list and per-host limits apply to every host.
func fetch(ctx context.Context, d Doer, origin string) ([]byte, error) {
	target := origin
	for hop := 0; ; hop++ {
		resp, err := get(ctx, d, target)
		if err != nil {
			return nil, err
		}
		next, redirect, err := follow(resp, target)
		if !redirect {
			return read(resp)
		}
		resp.Body.Close()
		if err != nil || hop == maxRedirects {
			return nil, errors.Join(errors.New("too many or bad redirects"), err)
		}
		target = next
	}
}

func get(ctx context.Context, d Doer, target string) (*http.Response, error) {
	if !fetchable(target) {
		return nil, fmt.Errorf("only http and https addresses are fetched")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/jpeg, image/png, image/webp, image/*;q=0.8")
	return d.Do(req)
}

// follow reports whether resp is a redirect and where it points.
func follow(resp *http.Response, from string) (string, bool, error) {
	switch resp.StatusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
	default:
		return "", false, nil
	}
	base, _ := url.Parse(from) // fetched before, so it parses
	loc, err := base.Parse(resp.Header.Get("Location"))
	if err != nil || resp.Header.Get("Location") == "" {
		return "", true, errors.New("redirect without a usable Location")
	}
	return loc.String(), true, nil
}

func read(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return nil, ErrMissing
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("the provider answered HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSource+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxSource {
		return nil, fmt.Errorf("%w: over %d MiB", ErrTooLarge, maxSource>>20)
	}
	return body, nil
}
