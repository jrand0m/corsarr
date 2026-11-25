package qbittorrent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/plugin"
)

func hashes(id string) url.Values { return url.Values{"hashes": {strings.ToLower(id)}} }

// Remove deletes a torrent, with or without its files. Removing an unknown
// hash succeeds, so a repeat after a crash ends in the same state.
func (c *Client) Remove(ctx context.Context, id string, deleteFiles bool) error {
	form := hashes(id)
	form.Set("deleteFiles", strconv.FormatBool(deleteFiles))
	_, err := c.postForm(ctx, "torrents/delete", form)
	return err
}

// Pause stops a torrent: `stop` on 5.x, `pause` on 4.x.
func (c *Client) Pause(ctx context.Context, id string) error {
	return c.pauseResume(ctx, id, "stop", "pause")
}

// Resume starts a torrent: `start` on 5.x, `resume` on 4.x.
func (c *Client) Resume(ctx context.Context, id string) error {
	return c.pauseResume(ctx, id, "start", "resume")
}

func (c *Client) pauseResume(ctx context.Context, id, v5, v4 string) error {
	major, err := c.majorVersion(ctx)
	if err != nil {
		return err
	}
	path := "torrents/" + v4
	if major >= 5 {
		path = "torrents/" + v5
	}
	_, err = c.postForm(ctx, path, hashes(id))
	return err
}

var priorityPaths = map[plugin.Priority]string{
	plugin.PrioTop: "topPrio", plugin.PrioBottom: "bottomPrio", plugin.PrioUp: "increasePrio", plugin.PrioDown: "decreasePrio",
}

// SetPriority moves a torrent in the client queue (requires queueing to be enabled in qBittorrent).
func (c *Client) SetPriority(ctx context.Context, id string, p plugin.Priority) error {
	path, ok := priorityPaths[p]
	if !ok {
		return plugin.Errorf(plugin.ClassConfig, "unknown priority %q", p)
	}
	_, err := c.postForm(ctx, "torrents/"+path, hashes(id))
	if hs := new(httpStatus); errors.As(err, hs) && hs.code == http.StatusConflict {
		return plugin.Errorf(plugin.ClassConfig, "enable torrent queueing in qBittorrent to change priorities")
	}
	return err
}

type categoryInfo struct {
	SavePath string `json:"savePath"`
}

// EnsureCategory creates the category or aligns its save path; calling it
// again changes nothing. The save path is local and sent in qBittorrent's
// view (pathMap).
func (c *Client) EnsureCategory(ctx context.Context, category, savePath string) error {
	savePath = c.remote(savePath)
	resp, err := c.get(ctx, "torrents/categories", nil, maxSmall)
	if err != nil {
		return err
	}
	var have map[string]categoryInfo
	if err := json.Unmarshal(resp.Body, &have); err != nil {
		return plugin.Wrap(plugin.ClassParse, "torrents/categories", err)
	}
	cur, ok := have[category]
	form := url.Values{"category": {category}, "savePath": {savePath}}
	switch {
	case !ok:
		_, err = c.postForm(ctx, "torrents/createCategory", form)
	case savePath != "" && cur.SavePath != savePath:
		_, err = c.postForm(ctx, "torrents/editCategory", form)
	}
	return err
}
