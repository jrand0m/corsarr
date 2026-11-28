package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jrand0m/corsarr/plugins/rtorrent/client"
	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
)

// module is the rTorrent download client: the downloader core and every
// optional capability, each a thin mapping onto the XML-RPC client.
type module struct{}

// cached is the client of the current settings. Keeping it keeps what the
// client learned about the server: methods it lacks and whether a front end
// can erase data.
var cached struct {
	key string
	c   *client.Client
}

func conn() (*client.Client, error) {
	values, err := plugin.Settings()
	if err != nil {
		return nil, err
	}
	password, err := plugin.Secret("password")
	if err != nil {
		return nil, err
	}
	values["password"] = password
	key, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	if cached.c != nil && cached.key == string(key) {
		return cached.c, nil
	}
	cfg, err := client.ConfigFrom(values)
	if err != nil {
		return nil, err
	}
	cfg.MaxResponse = plugin.DefaultMaxResponse
	c, err := client.New(plugin.Doer{}, cfg)
	if err != nil {
		return nil, err
	}
	cached.key, cached.c = string(key), c
	return c, nil
}

func with(fn func(ctx context.Context, c *client.Client) error) error {
	c, err := conn()
	if err != nil {
		return classify(err)
	}
	return classify(fn(context.Background(), c))
}

func onID(id string, fn func(c *client.Client, ctx context.Context, hash string) error) error {
	return with(func(ctx context.Context, c *client.Client) error { return fn(c, ctx, id) })
}

func (module) Test() error {
	return with(func(ctx context.Context, c *client.Client) error { _, err := c.Version(ctx); return err })
}

func (module) Protocols() []string { return []string{"torrent"} }

// KeepsDataOnRemove declares that corsarr removes rTorrent downloads without
// their data: rTorrent never deletes data itself, and only some servers have
// a front end's data-erasing handler.
func (module) KeepsDataOnRemove() bool { return true }

func (module) Add(r abi.AddRequest) (string, error) {
	req := client.AddRequest{Torrent: r.Payload.Content, Directory: r.SavePath, Label: r.Category, Paused: r.Paused}
	switch {
	case len(r.Payload.Content) > 0:
	case strings.HasPrefix(r.Payload.URL, "magnet:"):
		req.Magnet = r.Payload.URL
	default:
		return "", plugin.Errorf(abi.ClassConfig, "rTorrent needs the torrent file or a magnet link; this module cannot fetch %q", r.Payload.URL)
	}
	var id string
	err := with(func(ctx context.Context, c *client.Client) (err error) { id, err = c.Add(ctx, req); return err })
	return id, err
}

func (module) List(categories []string) ([]abi.Snapshot, error) {
	var out []abi.Snapshot
	err := with(func(ctx context.Context, c *client.Client) error {
		list, err := c.List(ctx, categories)
		out = make([]abi.Snapshot, len(list))
		for i, t := range list {
			out[i] = snapshot(t)
		}
		return err
	})
	return out, err
}

// queueStates fold rTorrent's states onto corsarr's queue states.
var queueStates = map[client.State]string{
	client.StateQueued: "queued", client.StatePaused: "queued", client.StateDownloading: "downloading",
	client.StateStalled: "stalled", client.StateSeeding: "completed", client.StateCompleted: "completed",
	client.StateChecking: "downloading", client.StateFailed: "stalled",
}

// snapshot reports a download in corsarr's queue states. A download rTorrent
// stopped by itself with a message is stalled with that message as the
// reason: the cause (a full disk, a missing mount) is outside the download,
// so it must not be blocklisted.
func snapshot(t client.Torrent) abi.Snapshot {
	s := abi.Snapshot{
		ID: t.Hash, Name: t.Name, Category: t.Label, State: queueStates[t.State], Progress: t.Progress,
		Size: t.Size, Downloaded: t.Done, Speed: t.DownRate, ETASeconds: t.ETASeconds, UpSpeed: t.UpRate,
		Ratio: t.Ratio, Seeders: t.Seeders, Peers: t.Peers, SavePath: t.SavePath, ContentPath: t.ContentPath,
	}
	if s.State == "" {
		s.State = "queued"
	}
	if t.State == client.StateChecking && t.Complete {
		s.State = "completed"
	}
	if t.State == client.StateFailed {
		s.Error = "rTorrent stopped this download: " + t.Message + "; fix the cause and start it again in rTorrent"
	}
	if s.State == "completed" {
		s.Files = []string{t.ContentPath}
	}
	return s
}

func (module) Remove(id string, deleteFiles bool) error {
	return with(func(ctx context.Context, c *client.Client) error { return c.Remove(ctx, id, deleteFiles) })
}

func (module) Pause(id string) error      { return onID(id, (*client.Client).Pause) }
func (module) Resume(id string) error     { return onID(id, (*client.Client).Resume) }
func (module) Recheck(id string) error    { return onID(id, (*client.Client).CheckHash) }
func (module) Reannounce(id string) error { return onID(id, (*client.Client).Reannounce) }

func (module) SetPriority(id, move string) error {
	return with(func(ctx context.Context, c *client.Client) error { return c.Move(ctx, id, move) })
}

func (module) SetPriorityLevel(id, level string) error {
	return with(func(ctx context.Context, c *client.Client) error {
		p, err := client.Level(level)
		if err != nil {
			return err
		}
		return c.SetPriority(ctx, id, p)
	})
}

// EnsureCategory has nothing to create: rTorrent labels are free-form.
func (module) EnsureCategory(category, _ string) error {
	return with(func(ctx context.Context, c *client.Client) error { return c.EnsureLabel(ctx, category) })
}

func (module) SetCategory(id, category string) error {
	return with(func(ctx context.Context, c *client.Client) error { return c.SetLabel(ctx, id, category) })
}

func (module) Files(id string) ([]abi.File, error) {
	var out []abi.File
	err := with(func(ctx context.Context, c *client.Client) error {
		files, err := c.Files(ctx, id)
		out = make([]abi.File, len(files))
		for i, f := range files {
			out[i] = abi.File{Path: f.Path, Size: f.Size}
		}
		return err
	})
	return out, err
}

func (module) Trackers(id string) ([]abi.Tracker, error) {
	var out []abi.Tracker
	err := with(func(ctx context.Context, c *client.Client) error {
		list, err := c.Trackers(ctx, id)
		out = make([]abi.Tracker, len(list))
		for i, t := range list {
			out[i] = abi.Tracker{URL: t.URL, Status: t.Status, Seeds: t.Seeds, Peers: t.Peers}
		}
		return err
	})
	return out, err
}

func (module) PauseAll() error {
	return with(func(ctx context.Context, c *client.Client) error { return c.PauseAll(ctx) })
}

func (module) ResumeAll() error {
	return with(func(ctx context.Context, c *client.Client) error { return c.ResumeAll(ctx) })
}

func (module) SpeedLimits() (abi.SpeedLimits, error) {
	var l client.Limits
	err := with(func(ctx context.Context, c *client.Client) (err error) { l, err = c.Limits(ctx); return err })
	return abi.SpeedLimits{DownKiB: l.Down / 1024, UpKiB: l.Up / 1024}, err
}

func (module) SetSpeedLimits(l abi.SpeedLimits) error {
	return with(func(ctx context.Context, c *client.Client) error {
		return c.SetLimits(ctx, client.Limits{Down: l.DownKiB * 1024, Up: l.UpKiB * 1024})
	})
}

// AltSpeed is always off: rTorrent has no alternative speed limits.
func (module) AltSpeed() (bool, error) { return false, nil }

// SetAltSpeed accepts off and answers unsupported for on.
func (module) SetAltSpeed(on bool) error {
	if on {
		return plugin.Unsupported("alternative speed limits in rTorrent")
	}
	return nil
}

func (module) Stats() (abi.Stats, error) {
	var s client.Stats
	err := with(func(ctx context.Context, c *client.Client) (err error) { s, err = c.Stats(ctx); return err })
	return abi.Stats{DownSpeed: s.DownRate, UpSpeed: s.UpRate, FreeSpace: max(s.FreeSpace, 0), Paused: s.Paused,
		Limits: abi.SpeedLimits{DownKiB: s.Limits.Down / 1024, UpKiB: s.Limits.Up / 1024}}, err
}
