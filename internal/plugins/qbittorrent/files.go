package qbittorrent

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/jrand0m/corsarr/internal/plugin"
)

var _ plugin.FileLister = (*Client)(nil)

type fileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Files returns the torrent's file list; it is empty while the metadata of a
// magnet is still being fetched.
func (c *Client) Files(ctx context.Context, id string) ([]plugin.FileEntry, error) {
	resp, err := c.get(ctx, "torrents/files", url.Values{"hash": {strings.ToLower(id)}}, maxList)
	if err != nil {
		return nil, err
	}
	var infos []fileInfo
	if err := json.Unmarshal(resp.Body, &infos); err != nil {
		return nil, plugin.Wrap(plugin.ClassParse, "torrents/files", err)
	}
	out := make([]plugin.FileEntry, len(infos))
	for i, f := range infos {
		out[i] = plugin.FileEntry{Path: f.Name, Size: f.Size}
	}
	return out, nil
}
