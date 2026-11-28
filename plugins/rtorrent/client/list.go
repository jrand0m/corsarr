package client

import (
	"context"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// State is a download's state in the host's terms.
type State string

const (
	StateQueued      State = "queued"
	StateDownloading State = "downloading"
	StateStalled     State = "stalled"
	StatePaused      State = "paused"
	StateCompleted   State = "completed"
	StateSeeding     State = "seeding"
	StateChecking    State = "checking"
	// StateFailed is a download rTorrent stopped by itself with a message
	// (usually a storage error) before it was complete. The cause tends to
	// be outside the download (a full disk, a missing mount), so a host may
	// prefer to show it as stalled with Message as the reason.
	StateFailed State = "failed"
)

// Torrent is one download as rTorrent reports it. Paths are in the caller's
// view (PathMap applied).
type Torrent struct {
	Hash        string // upper-case hex info hash
	Name        string
	Label       string
	State       State
	Message     string // rTorrent's last message: tracker or storage trouble
	Size        int64
	Done        int64 // bytes completed
	Left        int64
	Progress    float64 // 0..1
	DownRate    int64   // bytes/s
	UpRate      int64   // bytes/s
	Ratio       float64
	Seeders     int
	Peers       int
	ETASeconds  int64 // 0 when unknown
	Priority    Priority
	SavePath    string // the folder holding the payload
	ContentPath string // the payload's file or root folder
	MultiFile   bool

	Started  bool // d.state
	Active   bool // d.is_active: false while paused
	Open     bool // d.is_open
	Complete bool // d.complete
	Checking bool // d.is_hash_checking
}

type listField struct {
	cmd string
	set func(t *Torrent, v any)
}

// listFields are the per-download values one d.multicall2 reads; the label
// command is put in front by the client because its field is configurable.
var listFields = []listField{
	{"d.hash=", func(t *Torrent, v any) { t.Hash = strings.ToUpper(str(v)) }},
	{"d.name=", func(t *Torrent, v any) { t.Name = str(v) }},
	{"d.state=", func(t *Torrent, v any) { t.Started = num(v) != 0 }},
	{"d.is_active=", func(t *Torrent, v any) { t.Active = num(v) != 0 }},
	{"d.is_open=", func(t *Torrent, v any) { t.Open = num(v) != 0 }},
	{"d.complete=", func(t *Torrent, v any) { t.Complete = num(v) != 0 }},
	{"d.is_hash_checking=", func(t *Torrent, v any) { t.Checking = num(v) != 0 }},
	{"d.message=", func(t *Torrent, v any) { t.Message = str(v) }},
	{"d.size_bytes=", func(t *Torrent, v any) { t.Size = num(v) }},
	{"d.completed_bytes=", func(t *Torrent, v any) { t.Done = num(v) }},
	{"d.left_bytes=", func(t *Torrent, v any) { t.Left = num(v) }},
	{"d.down.rate=", func(t *Torrent, v any) { t.DownRate = num(v) }},
	{"d.up.rate=", func(t *Torrent, v any) { t.UpRate = num(v) }},
	{"d.ratio=", func(t *Torrent, v any) { t.Ratio = float64(num(v)) / 1000 }},
	{"d.peers_complete=", func(t *Torrent, v any) { t.Seeders = int(num(v)) }},
	{"d.peers_accounted=", func(t *Torrent, v any) { t.Peers = int(num(v)) }},
	{"d.priority=", func(t *Torrent, v any) { t.Priority = Priority(num(v)) }},
	{"d.is_multi_file=", func(t *Torrent, v any) { t.MultiFile = num(v) != 0 }},
	{"d.directory=", func(t *Torrent, v any) { t.ContentPath = str(v) }},
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// num reads an integer value; some proxies hand numbers back as strings.
func num(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case bool:
		if x {
			return 1
		}
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	}
	return 0
}

// stateOf maps rTorrent's flags onto a State. A started download that is
// not open is held back by a queue manager; one that is open but inactive is
// paused.
func stateOf(t *Torrent) State {
	switch {
	case t.Checking:
		return StateChecking
	case !t.Started:
		return stoppedState(t)
	case !t.Open:
		return StateQueued
	case !t.Active:
		return StatePaused
	case t.Complete:
		return StateSeeding
	case t.DownRate > 0:
		return StateDownloading
	}
	return StateStalled
}

func stoppedState(t *Torrent) State {
	switch {
	case t.Complete:
		return StateCompleted
	case t.Message != "":
		return StateFailed
	}
	return StatePaused
}

// multicall runs d.multicall2 over a view with the given commands. A build
// without d.multicall2 gets the older d.multicall, which takes no target.
func (c *Client) multicall(ctx context.Context, view string, cmds ...string) ([][]any, error) {
	params := make([]any, 0, len(cmds)+2)
	params = append(params, "", view)
	for _, cmd := range cmds {
		params = append(params, cmd)
	}
	v, err := c.Call(ctx, "d.multicall2", params...)
	if KindOf(err) == KindUnsupported {
		v, err = c.Call(ctx, "d.multicall", params[1:]...)
	}
	if err != nil {
		return nil, err
	}
	return rows(v, len(cmds))
}

// rows checks a multicall answer: a list of rows of n values each.
func rows(v any, n int) ([][]any, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, malformed("multicall answer is not a list")
	}
	out := make([][]any, len(list))
	for i, r := range list {
		row, ok := r.([]any)
		if !ok || len(row) != n {
			return nil, malformed("multicall row %d has the wrong shape", i)
		}
		out[i] = row
	}
	return out, nil
}

func (c *Client) labelCmd() string { return "d." + c.cfg.LabelField + "=" }

// List returns every download whose label is one of labels (every download
// when labels is empty) in one d.multicall2 round trip.
func (c *Client) List(ctx context.Context, labels []string) ([]Torrent, error) {
	cmds := make([]string, 0, len(listFields)+1)
	cmds = append(cmds, c.labelCmd())
	for _, f := range listFields {
		cmds = append(cmds, f.cmd)
	}
	rs, err := c.multicall(ctx, "main", cmds...)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, l := range labels {
		want[l] = true
	}
	out := []Torrent{}
	for _, r := range rs {
		t := c.torrent(r)
		if len(want) == 0 || want[t.Label] {
			out = append(out, t)
		}
	}
	return out, nil
}

func (c *Client) torrent(r []any) Torrent {
	t := Torrent{Label: decodeLabel(str(r[0]))}
	for i, f := range listFields {
		f.set(&t, r[i+1])
	}
	t.State = stateOf(&t)
	if t.Size > 0 {
		t.Progress = float64(t.Done) / float64(t.Size)
	}
	if t.DownRate > 0 && t.Left > 0 {
		t.ETASeconds = t.Left / t.DownRate
	}
	t.SavePath, t.ContentPath = payloadPaths(t.ContentPath, t.Name, t.MultiFile)
	t.SavePath = c.cfg.PathMap.ToLocal(t.SavePath)
	t.ContentPath = c.cfg.PathMap.ToLocal(t.ContentPath)
	return t
}

// payloadPaths derives the save folder and payload path from d.directory,
// which names the torrent's own folder for a multi-file torrent and the
// folder holding the file otherwise.
func payloadPaths(dir, name string, multi bool) (save, content string) {
	if dir == "" {
		return "", ""
	}
	if multi {
		return path.Dir(dir), dir
	}
	return dir, path.Join(dir, name)
}

// Labels are stored percent-encoded, the convention web front ends use in
// the custom fields; a stored value that does not decode is shown as is.
func decodeLabel(s string) string {
	if d, err := url.PathUnescape(s); err == nil {
		return d
	}
	return s
}

func encodeLabel(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }

// Exists reports whether rTorrent holds a download with this info hash.
func (c *Client) Exists(ctx context.Context, hash string) (bool, error) {
	_, err := c.Call(ctx, "d.hash", strings.ToUpper(hash))
	switch KindOf(err) {
	case "":
		return true, nil
	case KindNotFound:
		return false, nil
	}
	return false, err
}
