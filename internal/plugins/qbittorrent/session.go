package qbittorrent

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// call sends a request in an authenticated session. A 403 means the session
// expired: log in again and retry exactly once.
func (c *Client) call(ctx context.Context, max int64, mk func() (*http.Request, error)) (*plugin.Response, error) {
	resp, err := c.send(ctx, max, mk)
	if err == nil && resp.Status == http.StatusForbidden {
		c.reset()
		resp, err = c.send(ctx, max, mk)
	}
	if err != nil {
		return nil, err
	}
	if class := plugin.ClassifyStatus(resp.Status); class != "" {
		return nil, plugin.Errorf(class, "qBittorrent answered HTTP %d", resp.Status)
	}
	if resp.Status >= 400 {
		return nil, plugin.Wrap(plugin.ClassServerError, "qBittorrent refused the request", httpStatus{resp.Status})
	}
	return resp, nil
}

// httpStatus carries an unclassified error status for callers that care (409).
type httpStatus struct{ code int }

func (h httpStatus) Error() string { return "HTTP " + strconv.Itoa(h.code) }

func (c *Client) send(ctx context.Context, max int64, mk func() (*http.Request, error)) (*plugin.Response, error) {
	cookie, err := c.session(ctx)
	if err != nil {
		return nil, err
	}
	req, err := mk()
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassConfig, "building request", err)
	}
	c.decorate(req, cookie)
	return plugin.Do(c.inst.Env.HTTP(), req, max)
}

// decorate adds what the Web UI's CSRF check and session need.
func (c *Client) decorate(req *http.Request, cookie string) {
	req.Header.Set("Referer", c.base.String())
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
}

func (c *Client) reset() {
	c.mu.Lock()
	c.loggedIn, c.cookie = false, ""
	c.mu.Unlock()
}

// session returns the session cookie, logging in when needed.
func (c *Client) session(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loggedIn {
		return c.cookie, nil
	}
	cookie, err := c.login(ctx)
	if err != nil {
		return "", err
	}
	c.loggedIn, c.cookie = true, cookie
	return cookie, nil
}

// login posts the credentials. 4.x names the cookie SID and 5.x QBT_SID_<port>;
// both are taken as-is. With authentication bypassed there is no cookie.
func (c *Client) login(ctx context.Context) (string, error) {
	form := url.Values{"username": {c.inst.Settings.String("username")}, "password": {c.inst.Settings.String("password")}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("auth/login", nil), strings.NewReader(form.Encode()))
	if err != nil {
		return "", plugin.Wrap(plugin.ClassConfig, "building login request", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.decorate(req, "")
	resp, err := plugin.Do(c.inst.Env.HTTP(), req, maxSmall)
	if err != nil {
		return "", err
	}
	if class := plugin.ClassifyStatus(resp.Status); class != "" {
		return "", plugin.Errorf(class, "qBittorrent login answered HTTP %d (locked out after failed logins?)", resp.Status)
	}
	return sessionCookie(resp)
}

func sessionCookie(resp *plugin.Response) (string, error) {
	body := strings.TrimSpace(string(resp.Body))
	if strings.EqualFold(body, "Fails.") {
		return "", plugin.Errorf(plugin.ClassAuth, "qBittorrent rejected the user name or password")
	}
	// Older releases answer 200 "Ok."; newer ones answer 204 with an empty body.
	ok := (resp.Status == http.StatusOK && strings.EqualFold(body, "Ok.")) || (resp.Status == http.StatusNoContent && body == "")
	if !ok {
		return "", plugin.Errorf(plugin.ClassParse, "unexpected login answer (HTTP %d)", resp.Status)
	}
	for _, ck := range (&http.Response{Header: resp.Header}).Cookies() {
		if ck.Name == "SID" || strings.HasPrefix(ck.Name, "QBT_SID") {
			return ck.Name + "=" + ck.Value, nil
		}
	}
	return "", nil
}

// postForm sends an urlencoded POST.
func (c *Client) postForm(ctx context.Context, path string, form url.Values) (*plugin.Response, error) {
	body := form.Encode()
	return c.call(ctx, maxSmall, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(path, nil), strings.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return req, err
	})
}

// postBody sends a prebuilt body (multipart).
func (c *Client) postBody(ctx context.Context, path, contentType string, body []byte) (*plugin.Response, error) {
	return c.call(ctx, maxSmall, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(path, nil), bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", contentType)
		}
		return req, err
	})
}
