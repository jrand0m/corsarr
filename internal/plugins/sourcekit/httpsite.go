package sourcekit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// HTTPSite reaches a source through the source protocol: a small JSON API
// in front of the site (a companion service, or the site itself when it
// offers one).
//
//	GET {base}/search?artist=&album=&q=&format=  → {"items": [Item…]}
//	GET {base}/items/{id}/files?format=          → {"files": [FileRef…]}
//	GET <file url>                               → the file's bytes
//
// Every request carries the configured credential as a bearer token or
// cookie; file URLs on another host never receive it.
type HTTPSite struct {
	Base   string
	Token  string // sent as "Authorization: Bearer …" when set
	Cookie string // sent as the Cookie header when set
	Format string // preferred download format, passed through
	HTTP   plugin.HTTPDoer
	Name   string // plugin name for error messages
}

const maxJSON = 4 << 20

// ParseBase validates a source address once, at the boundary.
func ParseBase(name, base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", plugin.Errorf(plugin.ClassConfig, "%s baseUrl %q must be an http(s) address", name, base)
	}
	return strings.TrimRight(base, "/"), nil
}

// Test runs an empty search, which needs a working credential.
func (h HTTPSite) Test(ctx context.Context) error {
	_, err := h.Search(ctx, Query{Text: "test"})
	return err
}

// Search asks the source for albums.
func (h HTTPSite) Search(ctx context.Context, q Query) ([]Item, error) {
	v := url.Values{}
	for k, val := range map[string]string{"artist": q.Artist, "album": q.Album, "q": q.Text, "format": h.Format} {
		if val != "" {
			v.Set(k, val)
		}
	}
	var doc struct {
		Items []Item `json:"items"`
	}
	return doc.Items, h.getJSON(ctx, "/search?"+v.Encode(), &doc)
}

// Files lists one album's files.
func (h HTTPSite) Files(ctx context.Context, itemID string) ([]FileRef, error) {
	q := ""
	if h.Format != "" {
		q = "?" + url.Values{"format": {h.Format}}.Encode()
	}
	var doc struct {
		Files []FileRef `json:"files"`
	}
	return doc.Files, h.getJSON(ctx, "/items/"+url.PathEscape(itemID)+"/files"+q, &doc)
}

// Fetch opens one file.
func (h HTTPSite) Fetch(ctx context.Context, f FileRef) (Body, error) {
	target, err := url.Parse(f.URL)
	base, _ := url.Parse(h.Base)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, plugin.Errorf(plugin.ClassParse, "%s listed an invalid file address", h.Name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassConfig, "building request", err)
	}
	if target.Host == base.Host {
		h.authorize(req)
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassifyTransport(err), h.Name+" file", err)
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, plugin.Errorf(statusClass(resp.StatusCode), "%s file answered HTTP %d", h.Name, resp.StatusCode)
	}
	return resp.Body, nil
}

func (h HTTPSite) authorize(req *http.Request) {
	if h.Token != "" {
		req.Header.Set("Authorization", "Bearer "+h.Token)
	}
	if h.Cookie != "" {
		req.Header.Set("Cookie", h.Cookie)
	}
}

func statusClass(status int) plugin.FailureClass {
	if c := plugin.ClassifyStatus(status); c != "" {
		return c
	}
	return plugin.ClassServerError
}

func (h HTTPSite) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.Base+path, nil)
	if err != nil {
		return plugin.Wrap(plugin.ClassConfig, "building request", err)
	}
	h.authorize(req)
	req.Header.Set("Accept", "application/json")
	resp, err := plugin.Do(h.HTTP, req, maxJSON)
	if err != nil {
		return err
	}
	if resp.Status >= 400 {
		return plugin.Errorf(statusClass(resp.Status), "%s answered HTTP %d", h.Name, resp.Status)
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		return plugin.Wrap(plugin.ClassParse, fmt.Sprintf("%s %s", h.Name, strings.SplitN(path, "?", 2)[0]), err)
	}
	return nil
}
