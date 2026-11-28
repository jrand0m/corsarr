package client

import (
	"context"
	"net/url"
	"strings"
)

// PauseAll pauses every started download in one d.multicall2 over the
// "started" view. Paused downloads stay started, so ResumeAll brings back
// exactly these and leaves downloads stopped on purpose alone.
func (c *Client) PauseAll(ctx context.Context) error {
	_, err := c.multicall(ctx, "started", "d.pause=")
	return err
}

// ResumeAll resumes every paused download in the "started" view.
func (c *Client) ResumeAll(ctx context.Context) error {
	_, err := c.multicall(ctx, "started", "d.resume=")
	return err
}

// Limits are global transfer limits in bytes/s; 0 is unlimited.
type Limits struct {
	Down int64
	Up   int64
}

// Limits reads the global download and upload limits.
func (c *Client) Limits(ctx context.Context) (Limits, error) {
	v, err := c.Batch(ctx, []Request{
		{Method: "throttle.global_down.max_rate", Params: []any{""}},
		{Method: "throttle.global_up.max_rate", Params: []any{""}},
	})
	if err != nil {
		return Limits{}, err
	}
	return Limits{Down: num(v[0]), Up: num(v[1])}, nil
}

// SetLimits sets the global download and upload limits.
func (c *Client) SetLimits(ctx context.Context, l Limits) error {
	if l.Down < 0 || l.Up < 0 {
		return errorf(KindConfig, "speed limits must be 0 (unlimited) or more")
	}
	_, err := c.Batch(ctx, []Request{
		{Method: "throttle.global_down.max_rate.set", Params: []any{"", l.Down}},
		{Method: "throttle.global_up.max_rate.set", Params: []any{"", l.Up}},
	})
	return err
}

// Version is what the server reports about itself.
type Version struct {
	Client  string // rTorrent
	Library string // libtorrent
}

// Version reads the rTorrent and libtorrent versions; it doubles as the
// connection test.
func (c *Client) Version(ctx context.Context) (Version, error) {
	v, err := c.Batch(ctx, []Request{{Method: "system.client_version"}, {Method: "system.library_version"}})
	if err != nil {
		return Version{}, err
	}
	return Version{Client: str(v[0]), Library: str(v[1])}, nil
}

// FreeSpace returns the bytes free where downloads are saved, or -1 when
// unknown. rTorrent reports free space per download (d.free_diskspace), so
// a download in the default directory is preferred and any download is the
// fallback; with no downloads the answer is unknown.
func (c *Client) FreeSpace(ctx context.Context) (int64, error) {
	_, free, err := c.disk(ctx)
	return free, err
}

// disk reads every download's activity and free space plus the default
// directory, and returns whether any download is active and the free space.
func (c *Client) disk(ctx context.Context) (active bool, free int64, err error) {
	v, err := c.Call(ctx, "directory.default")
	if err != nil {
		return false, -1, err
	}
	rs, err := c.multicall(ctx, "main", "d.is_active=", "d.directory=", "d.free_diskspace=")
	if err != nil {
		return false, -1, err
	}
	def := strings.TrimRight(str(v), "/")
	free = -1
	for _, r := range rs {
		active = active || num(r[0]) != 0
		if free < 0 || (def != "" && strings.HasPrefix(str(r[1]), def)) {
			free = num(r[2])
		}
	}
	return active, free, nil
}

// Stats is the client-wide state at one moment.
type Stats struct {
	DownRate  int64 // bytes/s
	UpRate    int64 // bytes/s
	Limits    Limits
	FreeSpace int64 // -1 unknown
	// Paused is true when there are downloads and none of them is active.
	Paused bool
}

// Stats reads speeds, limits, free space and whether everything is paused.
func (c *Client) Stats(ctx context.Context) (Stats, error) {
	v, err := c.Batch(ctx, []Request{
		{Method: "throttle.global_down.rate", Params: []any{""}},
		{Method: "throttle.global_up.rate", Params: []any{""}},
		{Method: "throttle.global_down.max_rate", Params: []any{""}},
		{Method: "throttle.global_up.max_rate", Params: []any{""}},
	})
	if err != nil {
		return Stats{}, err
	}
	active, free, err := c.disk(ctx)
	if err != nil {
		return Stats{}, err
	}
	return Stats{
		DownRate: num(v[0]), UpRate: num(v[1]), Limits: Limits{Down: num(v[2]), Up: num(v[3])},
		FreeSpace: free, Paused: free >= 0 && !active, // free space is known only with downloads
	}, nil
}

// Tracker is one tracker of a download. URL holds only scheme and host:
// private trackers put the user's passkey in the announce path or query.
type Tracker struct {
	URL    string
	Status string // working | not_contacted | not_working | disabled
	Seeds  int
	Peers  int
}

var trackerCmds = []string{"t.url=", "t.is_enabled=", "t.scrape_complete=", "t.scrape_incomplete=",
	"t.success_counter=", "t.failed_counter="}

// Trackers lists a download's trackers (t.multicall); DHT pseudo entries
// are left out.
func (c *Client) Trackers(ctx context.Context, hash string) ([]Tracker, error) {
	params := []any{id(hash), ""}
	for _, cmd := range trackerCmds {
		params = append(params, cmd)
	}
	v, err := c.Call(ctx, "t.multicall", params...)
	if err != nil {
		return nil, err
	}
	rs, err := rows(v, len(trackerCmds))
	if err != nil {
		return nil, err
	}
	out := []Tracker{}
	for _, r := range rs {
		if strings.HasPrefix(str(r[0]), "dht://") {
			continue
		}
		out = append(out, Tracker{URL: TrackerOrigin(str(r[0])), Status: trackerStatus(r),
			Seeds: int(num(r[2])), Peers: int(num(r[3]))})
	}
	return out, nil
}

func trackerStatus(r []any) string {
	switch {
	case num(r[1]) == 0:
		return "disabled"
	case num(r[5]) > 0:
		return "not_working"
	case num(r[4]) > 0:
		return "working"
	}
	return "not_contacted"
}

// TrackerOrigin keeps only the scheme and host of a tracker address.
func TrackerOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "(unparsable tracker address)"
	}
	return u.Scheme + "://" + u.Host
}

// File is one file of a download's payload.
type File struct {
	Path     string // relative to the payload root
	Size     int64
	Progress float64 // 0..1
	Priority int     // 0 skip, 1 normal, 2 high
}

var fileCmds = []string{"f.path=", "f.size_bytes=", "f.completed_chunks=", "f.size_chunks=", "f.priority="}

// Files lists a download's files (f.multicall); empty while a magnet's
// metadata is still unknown.
func (c *Client) Files(ctx context.Context, hash string) ([]File, error) {
	params := []any{id(hash), ""}
	for _, cmd := range fileCmds {
		params = append(params, cmd)
	}
	v, err := c.Call(ctx, "f.multicall", params...)
	if err != nil {
		return nil, err
	}
	rs, err := rows(v, len(fileCmds))
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(rs))
	for _, r := range rs {
		f := File{Path: str(r[0]), Size: num(r[1]), Priority: int(num(r[4]))}
		if chunks := num(r[3]); chunks > 0 {
			f.Progress = float64(num(r[2])) / float64(chunks)
		}
		out = append(out, f)
	}
	return out, nil
}
