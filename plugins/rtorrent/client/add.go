package client

import (
	"context"
	"net/url"
	"strings"
)

// MaxTorrentFile caps a .torrent file fetched from a URL.
const MaxTorrentFile = 10 << 20

// AddRequest asks rTorrent to load a download. Exactly one of Magnet,
// Torrent or URL is used, in that order.
type AddRequest struct {
	Magnet  string // magnet link
	Torrent []byte // .torrent file contents
	URL     string // http(s) address of a .torrent file, fetched through the Doer
	// Directory is the save folder in the caller's view (PathMap applied);
	// empty keeps rTorrent's default.
	Directory string
	Label     string
	// Paused loads the download without starting it.
	Paused bool
}

// Add loads a download and returns its upper-case info hash. It is
// idempotent: a download rTorrent already holds is returned untouched.
// Magnets go through load.start (load.normal when paused), torrent bytes
// through load.raw_start (load.raw); directory and label are set by
// commands that run as the download is created.
func (c *Client) Add(ctx context.Context, req AddRequest) (string, error) {
	method, payload, hash, err := c.payload(ctx, req)
	if err != nil {
		return "", err
	}
	if have, err := c.Exists(ctx, hash); err != nil || have {
		return hash, err
	}
	params := append([]any{"", payload}, c.loadCommands(req)...)
	if _, err := c.Call(ctx, method, params...); err != nil {
		return "", err
	}
	have, err := c.Exists(ctx, hash)
	if err == nil && !have {
		err = errorf(KindServer, "rTorrent did not load the download")
	}
	return hash, err
}

// payload picks the load method and argument and works out the info hash.
func (c *Client) payload(ctx context.Context, req AddRequest) (method string, payload any, hash string, err error) {
	if req.Magnet == "" && strings.HasPrefix(req.URL, "magnet:") {
		req.Magnet = req.URL
	}
	switch {
	case req.Magnet != "":
		hash, err = MagnetHash(req.Magnet)
		return loadMethod("load.start", "load.normal", req.Paused), req.Magnet, hash, err
	case len(req.Torrent) == 0 && req.URL != "":
		if req.Torrent, err = c.fetch(ctx, req.URL); err != nil {
			return "", nil, "", err
		}
	case len(req.Torrent) == 0:
		return "", nil, "", errorf(KindConfig, "nothing to add: no magnet, torrent file or URL")
	}
	hash, err = TorrentHash(req.Torrent)
	return loadMethod("load.raw_start", "load.raw", req.Paused), req.Torrent, hash, err
}

func loadMethod(start, normal string, paused bool) string {
	if paused {
		return normal
	}
	return start
}

// fetch downloads a .torrent file through the Doer.
func (c *Client) fetch(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errorf(KindConfig, "torrent URL must be http(s) or a magnet link")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	status, _, body, err := c.doer.Do(ctx, "GET", raw, map[string]string{}, nil)
	if err != nil {
		return nil, wrap(classifyTransport(err), "fetching the torrent file", err)
	}
	if status < 200 || status >= 300 {
		return nil, errorf(KindServer, "fetching the torrent file: HTTP %d", status)
	}
	if len(body) > MaxTorrentFile {
		return nil, malformed("torrent file exceeds %d bytes", MaxTorrentFile)
	}
	return body, nil
}

// loadCommands are the commands run on the new download.
func (c *Client) loadCommands(req AddRequest) []any {
	var cmds []any
	if req.Directory != "" {
		cmds = append(cmds, "d.directory.set="+quoteArg(c.cfg.PathMap.ToRemote(req.Directory)))
	}
	if req.Label != "" {
		cmds = append(cmds, "d."+c.cfg.LabelField+".set="+quoteArg(encodeLabel(req.Label)))
	}
	return cmds
}

// quoteArg quotes a command argument so commas, spaces and quotes in it
// reach rTorrent's command parser as one string.
func quoteArg(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
