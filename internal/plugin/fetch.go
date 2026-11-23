package plugin

import (
	"errors"
	"net/http"
	"net/url"
)

// Response is a fully read, size-bounded HTTP response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Do sends req through the shared client and reads at most max bytes. Transport
// failures and oversize bodies are classified errors; HTTP statuses are not
// (use Fetch for the usual status mapping). Query strings (API keys) never
// appear in returned errors.
func Do(d HTTPDoer, req *http.Request, max int64) (*Response, error) {
	resp, err := d.Do(req)
	if err != nil {
		return nil, Wrap(ClassifyTransport(err), "request failed", scrub(err))
	}
	defer resp.Body.Close()
	body, err := ReadLimited(resp.Body, max+1)
	if err != nil {
		return nil, Wrap(ClassifyTransport(err), "reading response", scrub(err))
	}
	if int64(len(body)) > max {
		return nil, Errorf(ClassParse, "response exceeds %d bytes", max)
	}
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: body}, nil
}

// Fetch is Do plus the status mapping of ClassifyStatus.
func Fetch(d HTTPDoer, req *http.Request, max int64) (*Response, error) {
	r, err := Do(d, req, max)
	if err != nil {
		return nil, err
	}
	if c := ClassifyStatus(r.Status); c != "" {
		return nil, Errorf(c, "%s %s returned HTTP %d", req.Method, Redact(req.URL), r.Status)
	}
	return r, nil
}

// Redact renders a URL without credentials or query, safe for logs and errors.
func Redact(u *url.URL) string { return u.Scheme + "://" + u.Host + u.Path }

func scrub(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	u, perr := url.Parse(ue.URL)
	if perr != nil {
		return ue.Err
	}
	if u.Scheme == "magnet" { // carries no credentials; callers read it from redirects
		return err
	}
	c := *ue
	c.URL = Redact(u)
	return &c
}
