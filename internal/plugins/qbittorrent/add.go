package qbittorrent

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// Add starts a payload and returns its info hash. It is idempotent: a torrent
// the client already holds is returned without being added again. A save
// path is sent in qBittorrent's view (pathMap).
func (c *Client) Add(ctx context.Context, req plugin.AddRequest) (string, error) {
	req.SavePath = c.remote(req.SavePath)
	magnet, content, hash, err := c.prepare(ctx, req.Payload)
	if err != nil {
		return "", err
	}
	if exists, err := c.exists(ctx, hash); err != nil || exists {
		return hash, err
	}
	body, ctype, err := c.addForm(ctx, req, magnet, content)
	if err != nil {
		return "", err
	}
	resp, err := c.postBody(ctx, "torrents/add", ctype, body)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(strings.TrimSpace(string(resp.Body)), "Ok.") {
		return hash, nil
	}
	if exists, err := c.exists(ctx, hash); err != nil || exists {
		return hash, err
	}
	return "", plugin.Errorf(plugin.ClassParse, "qBittorrent refused the torrent")
}

// prepare reduces any payload to a magnet link or torrent bytes plus the info hash.
func (c *Client) prepare(ctx context.Context, p plugin.Payload) (magnet string, content []byte, hash string, err error) {
	switch {
	case strings.HasPrefix(p.URL, "magnet:"):
		hash, err = magnetHash(p.URL)
		return p.URL, nil, hash, err
	case len(p.Content) > 0:
		content = p.Content
	case p.URL != "":
		if content, err = c.download(ctx, p.URL); err != nil {
			return "", nil, "", err
		}
	default:
		return "", nil, "", plugin.Errorf(plugin.ClassParse, "payload has neither magnet, URL nor torrent bytes")
	}
	hash, err = torrentHash(content)
	return "", content, hash, err
}

// download fetches a .torrent URL through the shared client.
func (c *Client) download(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, plugin.Errorf(plugin.ClassParse, "payload URL must be http(s) or magnet")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, plugin.Wrap(plugin.ClassConfig, "building request", err)
	}
	resp, err := plugin.Fetch(c.inst.Env.HTTP(), req, maxFile)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) addForm(ctx context.Context, req plugin.AddRequest, magnet string, content []byte) ([]byte, string, error) {
	major, err := c.majorVersion(ctx)
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if magnet != "" {
		_ = w.WriteField("urls", magnet)
	} else {
		part, _ := w.CreateFormFile("torrents", "payload.torrent")
		_, _ = part.Write(content)
	}
	for k, v := range addFields(req, major) {
		_ = w.WriteField(k, v)
	}
	if err := w.Close(); err != nil {
		return nil, "", plugin.Wrap(plugin.ClassConfig, "building upload", err)
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// addFields are the plain form fields; 5.x renamed `paused` to `stopped`.
func addFields(req plugin.AddRequest, major int) map[string]string {
	f := map[string]string{}
	if req.Category != "" {
		f["category"] = req.Category
	}
	if req.SavePath != "" {
		f["savepath"] = req.SavePath
		f["autoTMM"] = "false"
	}
	if req.Paused && major >= 5 {
		f["stopped"] = "true"
	} else if req.Paused {
		f["paused"] = "true"
	}
	return f
}
