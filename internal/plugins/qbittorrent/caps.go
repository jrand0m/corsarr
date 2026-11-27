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

var (
	_ plugin.GlobalPauser  = (*Client)(nil)
	_ plugin.SpeedLimiter  = (*Client)(nil)
	_ plugin.Rechecker     = (*Client)(nil)
	_ plugin.Reannouncer   = (*Client)(nil)
	_ plugin.Categoriser   = (*Client)(nil)
	_ plugin.ClientStats   = (*Client)(nil)
	_ plugin.TrackerLister = (*Client)(nil)
)

// PauseAll stops every torrent.
func (c *Client) PauseAll(ctx context.Context) error { return c.Pause(ctx, "all") }

// ResumeAll starts every torrent.
func (c *Client) ResumeAll(ctx context.Context) error { return c.Resume(ctx, "all") }

// Recheck verifies a torrent's data again.
func (c *Client) Recheck(ctx context.Context, id string) error {
	_, err := c.postForm(ctx, "torrents/recheck", hashes(id))
	return err
}

// Reannounce contacts the torrent's trackers now.
func (c *Client) Reannounce(ctx context.Context, id string) error {
	_, err := c.postForm(ctx, "torrents/reannounce", hashes(id))
	return err
}

// SetCategory moves a torrent to an existing category ("" clears it).
func (c *Client) SetCategory(ctx context.Context, id, category string) error {
	form := hashes(id)
	form.Set("category", category)
	_, err := c.postForm(ctx, "torrents/setCategory", form)
	if hs := new(httpStatus); errors.As(err, hs) && hs.code == http.StatusConflict {
		return plugin.Errorf(plugin.ClassConfig, "qBittorrent has no category %q", category)
	}
	return err
}

// SpeedLimits reads the global download and upload limits.
func (c *Client) SpeedLimits(ctx context.Context) (plugin.SpeedLimits, error) {
	down, err := c.number(ctx, "transfer/downloadLimit")
	if err != nil {
		return plugin.SpeedLimits{}, err
	}
	up, err := c.number(ctx, "transfer/uploadLimit")
	return plugin.SpeedLimits{DownKiB: down / 1024, UpKiB: up / 1024}, err
}

// SetSpeedLimits sets the global download and upload limits (0 = unlimited).
func (c *Client) SetSpeedLimits(ctx context.Context, l plugin.SpeedLimits) error {
	if l.DownKiB < 0 || l.UpKiB < 0 {
		return plugin.Errorf(plugin.ClassConfig, "speed limits must be 0 (unlimited) or more")
	}
	if _, err := c.postForm(ctx, "transfer/setDownloadLimit", url.Values{"limit": {strconv.FormatInt(l.DownKiB*1024, 10)}}); err != nil {
		return err
	}
	_, err := c.postForm(ctx, "transfer/setUploadLimit", url.Values{"limit": {strconv.FormatInt(l.UpKiB*1024, 10)}})
	return err
}

// AltSpeed reports whether the alternative speed limits are in force.
func (c *Client) AltSpeed(ctx context.Context) (bool, error) {
	n, err := c.number(ctx, "transfer/speedLimitsMode")
	return n == 1, err
}

// SetAltSpeed turns the alternative speed limits on or off; qBittorrent only
// toggles, so the current mode is read first.
func (c *Client) SetAltSpeed(ctx context.Context, on bool) error {
	cur, err := c.AltSpeed(ctx)
	if err != nil || cur == on {
		return err
	}
	_, err = c.postForm(ctx, "transfer/toggleSpeedLimitsMode", url.Values{})
	return err
}

func (c *Client) number(ctx context.Context, path string) (int64, error) {
	resp, err := c.get(ctx, path, nil, maxSmall)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(resp.Body)), 10, 64)
	if err != nil {
		return 0, plugin.Wrap(plugin.ClassParse, path, err)
	}
	return n, nil
}

type mainData struct {
	ServerState struct {
		DLSpeed   int64  `json:"dl_info_speed"`
		UPSpeed   int64  `json:"up_info_speed"`
		FreeSpace *int64 `json:"free_space_on_disk"`
		DLLimit   int64  `json:"dl_rate_limit"`
		UPLimit   int64  `json:"up_rate_limit"`
		AltSpeed  bool   `json:"use_alt_speed_limits"`
	} `json:"server_state"`
	Torrents map[string]struct {
		State string `json:"state"`
	} `json:"torrents"`
}

// Stats reads speeds, free space, limits and whether every torrent is stopped
// in one sync call.
func (c *Client) Stats(ctx context.Context) (plugin.ClientStatus, error) {
	resp, err := c.get(ctx, "sync/maindata", url.Values{"rid": {"0"}}, maxList)
	if err != nil {
		return plugin.ClientStatus{}, err
	}
	var md mainData
	if err := json.Unmarshal(resp.Body, &md); err != nil {
		return plugin.ClientStatus{}, plugin.Wrap(plugin.ClassParse, "sync/maindata", err)
	}
	ss := md.ServerState
	st := plugin.ClientStatus{DownSpeed: ss.DLSpeed, UpSpeed: ss.UPSpeed, FreeSpace: -1, AltSpeed: ss.AltSpeed,
		Limits: plugin.SpeedLimits{DownKiB: ss.DLLimit / 1024, UpKiB: ss.UPLimit / 1024}}
	if ss.FreeSpace != nil {
		st.FreeSpace = *ss.FreeSpace
	}
	st.Paused = len(md.Torrents) > 0
	for _, t := range md.Torrents {
		if !strings.HasPrefix(t.State, "paused") && !strings.HasPrefix(t.State, "stopped") {
			st.Paused = false
		}
	}
	return st, nil
}

type trackerInfo struct {
	URL      string `json:"url"`
	Status   int    `json:"status"`
	Msg      string `json:"msg"`
	NumSeeds int    `json:"num_seeds"`
	NumPeers int    `json:"num_peers"`
}

var trackerStatus = map[int]string{0: "disabled", 1: "not_contacted", 2: "working", 3: "updating", 4: "not_working"}

// Trackers lists a torrent's trackers; the DHT, PeX and LSD pseudo entries
// are left out.
func (c *Client) Trackers(ctx context.Context, id string) ([]plugin.Tracker, error) {
	resp, err := c.get(ctx, "torrents/trackers", url.Values{"hash": {strings.ToLower(id)}}, maxList)
	if err != nil {
		return nil, err
	}
	var infos []trackerInfo
	if err := json.Unmarshal(resp.Body, &infos); err != nil {
		return nil, plugin.Wrap(plugin.ClassParse, "torrents/trackers", err)
	}
	out := []plugin.Tracker{}
	for _, t := range infos {
		if strings.HasPrefix(t.URL, "** [") {
			continue
		}
		out = append(out, plugin.Tracker{URL: trackerOrigin(t.URL), Status: trackerStatus[t.Status], Message: t.Msg,
			Seeds: t.NumSeeds, Peers: t.NumPeers})
	}
	return out, nil
}

// trackerOrigin keeps only scheme and host: private trackers put the user's
// passkey in the announce path or query.
func trackerOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "(unparsable tracker address)"
	}
	return u.Scheme + "://" + u.Host
}
