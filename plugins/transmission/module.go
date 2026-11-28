package main

import (
	"context"
	"encoding/json"

	"github.com/jrand0m/corsarr/plugins/transmission/client"
	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
)

// module is the Transmission download client: the downloader core and every
// optional capability, each a thin mapping onto the protocol client.
type module struct{}

// cached is the client of the current settings; a module instance serves one
// corsarr instance, so the cache only changes when the settings do. Keeping
// the client keeps its session id and what it learned about the server.
var cached struct {
	key string
	c   *client.Client
}

// conn returns the client for the instance's current settings.
func conn() (*client.Client, error) {
	values, err := plugin.Settings()
	if err != nil {
		return nil, err
	}
	password, err := plugin.Secret(client.KeyPassword)
	if err != nil {
		return nil, err
	}
	values[client.KeyPassword] = password
	key, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	if cached.c != nil && cached.key == string(key) {
		return cached.c, nil
	}
	cfg, err := client.ConfigFromSettings(values)
	if err != nil {
		return nil, err
	}
	c, err := client.New(plugin.Doer{}, cfg)
	if err != nil {
		return nil, err
	}
	cached.key, cached.c = string(key), c
	return c, nil
}

// with runs fn with the client and classifies its error.
func with(fn func(ctx context.Context, c *client.Client) error) error {
	c, err := conn()
	if err != nil {
		return classify(err)
	}
	return classify(fn(context.Background(), c))
}

func (module) Test() error {
	return with(func(ctx context.Context, c *client.Client) error { _, err := c.Test(ctx); return err })
}

func (module) Protocols() []string { return []string{"torrent"} }

func (module) Add(r abi.AddRequest) (string, error) {
	var id string
	err := with(func(ctx context.Context, c *client.Client) (err error) {
		id, err = c.Add(ctx, client.AddRequest{URL: r.Payload.URL, Torrent: r.Payload.Content,
			Category: r.Category, SavePath: r.SavePath, Paused: r.Paused})
		return err
	})
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

// snapshot reports a torrent in corsarr's queue states (the client's
// folding); a torrent with a local error is stalled with its reason.
func snapshot(t client.Torrent) abi.Snapshot {
	return abi.Snapshot{
		ID: t.ID, Name: t.Name, Category: t.Category, State: string(t.QueueState), Progress: t.Progress,
		Size: t.Size, Downloaded: t.Downloaded, Speed: t.DownSpeed, ETASeconds: t.ETASeconds,
		UpSpeed: t.UpSpeed, Ratio: t.Ratio, Seeders: t.Seeders, Peers: t.Peers,
		SavePath: t.SavePath, ContentPath: t.ContentPath, Files: t.Files,
		LastActivityAt: t.LastActivity, Error: t.Error,
	}
}

func onID(id string, fn func(c *client.Client, ctx context.Context, id string) error) error {
	return with(func(ctx context.Context, c *client.Client) error { return fn(c, ctx, id) })
}

func (module) Remove(id string, deleteFiles bool) error {
	return with(func(ctx context.Context, c *client.Client) error { return c.Remove(ctx, id, deleteFiles) })
}

func (module) Pause(id string) error      { return onID(id, (*client.Client).Pause) }
func (module) Resume(id string) error     { return onID(id, (*client.Client).Resume) }
func (module) Recheck(id string) error    { return onID(id, (*client.Client).Recheck) }
func (module) Reannounce(id string) error { return onID(id, (*client.Client).Reannounce) }

func (module) SetPriority(id, move string) error {
	return with(func(ctx context.Context, c *client.Client) error {
		return c.SetPriority(ctx, id, client.Priority(move))
	})
}

func (module) SetPriorityLevel(id, level string) error {
	return with(func(ctx context.Context, c *client.Client) error {
		return c.SetPriorityLevel(ctx, id, client.Priority(level))
	})
}

func (module) EnsureCategory(category, savePath string) error {
	return with(func(ctx context.Context, c *client.Client) error { return c.EnsureCategory(ctx, category, savePath) })
}

func (module) SetCategory(id, category string) error {
	return with(func(ctx context.Context, c *client.Client) error { return c.SetCategory(ctx, id, category) })
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
			out[i] = abi.Tracker{URL: t.URL, Status: t.Status, Message: t.Message, Seeds: t.Seeds, Peers: t.Peers}
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
	var l client.SpeedLimits
	err := with(func(ctx context.Context, c *client.Client) (err error) { l, err = c.SpeedLimits(ctx); return err })
	return abi.SpeedLimits{DownKiB: l.DownKiB, UpKiB: l.UpKiB}, err
}

func (module) SetSpeedLimits(l abi.SpeedLimits) error {
	return with(func(ctx context.Context, c *client.Client) error {
		return c.SetSpeedLimits(ctx, client.SpeedLimits{DownKiB: l.DownKiB, UpKiB: l.UpKiB})
	})
}

func (module) AltSpeed() (bool, error) {
	var on bool
	err := with(func(ctx context.Context, c *client.Client) (err error) { on, err = c.AltSpeed(ctx); return err })
	return on, err
}

func (module) SetAltSpeed(on bool) error {
	return with(func(ctx context.Context, c *client.Client) error { return c.SetAltSpeed(ctx, on) })
}

func (module) Stats() (abi.Stats, error) {
	var s client.Status
	err := with(func(ctx context.Context, c *client.Client) (err error) { s, err = c.Stats(ctx); return err })
	return abi.Stats{DownSpeed: s.DownSpeed, UpSpeed: s.UpSpeed, FreeSpace: max(s.FreeSpace, 0), Paused: s.Paused,
		Limits: abi.SpeedLimits{DownKiB: s.Limits.DownKiB, UpKiB: s.Limits.UpKiB}, AltSpeed: s.AltSpeed}, err
}
