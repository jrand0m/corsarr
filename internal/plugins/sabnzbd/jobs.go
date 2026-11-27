package sabnzbd

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"mime/multipart"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

type queueSlot struct {
	ID       string `json:"nzo_id"`
	Name     string `json:"filename"`
	Category string `json:"cat"`
	Status   string `json:"status"`
	Percent  num    `json:"percentage"`
	MB       num    `json:"mb"`
	MBLeft   num    `json:"mbleft"`
	TimeLeft string `json:"timeleft"`
	Index    int    `json:"index"`
}

type queueInfo struct {
	Paused        bool        `json:"paused"`
	KBPerSec      num         `json:"kbpersec"`
	SpeedLimitAbs num         `json:"speedlimit_abs"` // bytes/s; empty = none
	DiskSpace     num         `json:"diskspace1"`     // GB free in the download folder
	Slots         []queueSlot `json:"slots"`
}

type historySlot struct {
	ID       string `json:"nzo_id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Status   string `json:"status"`
	Storage  string `json:"storage"`
	Fail     string `json:"fail_message"`
	Bytes    num    `json:"bytes"`
}

const mib = 1 << 20

func (c *Client) queue(ctx context.Context, q url.Values) (queueInfo, error) {
	var out struct {
		Queue queueInfo `json:"queue"`
	}
	err := c.call(ctx, "queue", q, maxList, &out)
	return out.Queue, err
}

func (c *Client) history(ctx context.Context, q url.Values) ([]historySlot, error) {
	var out struct {
		History struct {
			Slots []historySlot `json:"slots"`
		} `json:"history"`
	}
	err := c.call(ctx, "history", q, maxList, &out)
	return out.History.Slots, err
}

// jobName is the name the job gets in SABnzbd: the payload's own name, else
// one derived from its URL or bytes, so a repeated add finds the first one.
func jobName(p plugin.Payload) string {
	if p.Name != "" {
		return p.Name
	}
	src := p.Content
	if len(src) == 0 {
		src = []byte(p.URL)
	}
	sum := sha1.Sum(src)
	return "corsarr-" + hex.EncodeToString(sum[:6])
}

// Add hands an NZB to the service and returns its job id. A job with the same
// name already in the queue or history is returned instead (idempotent).
func (c *Client) Add(ctx context.Context, req plugin.AddRequest) (string, error) {
	if req.Payload.URL == "" && len(req.Payload.Content) == 0 {
		return "", plugin.Errorf(plugin.ClassParse, "payload has neither URL nor NZB bytes")
	}
	name := jobName(req.Payload)
	if id, err := c.find(ctx, name); err != nil || id != "" {
		return id, err
	}
	q := url.Values{"nzbname": {name}, "cat": {req.Category}}
	if req.Paused {
		q.Set("priority", "-2")
	}
	var out struct {
		IDs []string `json:"nzo_ids"`
	}
	if err := c.send(ctx, req.Payload, q, &out); err != nil {
		return "", err
	}
	if len(out.IDs) == 0 {
		return "", plugin.Errorf(plugin.ClassParse, "SABnzbd accepted the NZB but named no job")
	}
	return out.IDs[0], nil
}

func (c *Client) send(ctx context.Context, p plugin.Payload, q url.Values, out any) error {
	if len(p.Content) == 0 {
		q.Set("name", p.URL)
		return c.call(ctx, "addurl", q, maxSmall, out)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("name", q.Get("nzbname")+".nzb") // writes to memory
	_, _ = fw.Write(p.Content)
	_ = mw.Close()
	return c.post(ctx, "addfile", q, mw.FormDataContentType(), buf.Bytes(), out)
}

// find returns the id of a job with this name in the queue or history.
func (c *Client) find(ctx context.Context, name string) (string, error) {
	qi, err := c.queue(ctx, nil)
	if err != nil {
		return "", err
	}
	for _, s := range qi.Slots {
		if s.Name == name {
			return s.ID, nil
		}
	}
	hist, err := c.history(ctx, url.Values{"search": {name}})
	for _, h := range hist {
		if h.Name == name {
			return h.ID, err
		}
	}
	return "", err
}

// List returns one snapshot per job in the categories (all when none are
// named): queued jobs first, then finished ones from the history.
func (c *Client) List(ctx context.Context, categories []string) ([]plugin.Snapshot, error) {
	qi, err := c.queue(ctx, nil)
	if err != nil {
		return nil, err
	}
	hist, err := c.history(ctx, url.Values{"limit": {"500"}})
	if err != nil {
		return nil, err
	}
	in := func(cat string) bool { return len(categories) == 0 || slices.Contains(categories, cat) }
	var out []plugin.Snapshot
	for _, s := range qi.Slots {
		if in(s.Category) {
			out = append(out, queueSnapshot(s))
		}
	}
	for _, h := range hist {
		if in(h.Category) {
			out = append(out, c.historySnapshot(h))
		}
	}
	return out, nil
}

var queueStates = map[string]domain.DownloadState{"Downloading": domain.StateDownloading}

func queueSnapshot(s queueSlot) plugin.Snapshot {
	st, ok := queueStates[s.Status]
	if !ok {
		st = domain.StateQueued // Queued, Paused, Grabbing, Fetching, Propagating
	}
	return plugin.Snapshot{DownloadID: s.ID, Name: s.Name, Category: s.Category, State: st,
		Progress: float64(s.Percent) / 100, Size: int64(s.MB * mib), Downloaded: int64((s.MB - s.MBLeft) * mib),
		ETASeconds: clock(s.TimeLeft)}
}

// clock parses SABnzbd's [D:]H:MM:SS time left.
func clock(s string) int64 {
	var total int64
	for _, part := range strings.Split(s, ":") {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return total
}

func (c *Client) historySnapshot(h historySlot) plugin.Snapshot {
	s := plugin.Snapshot{DownloadID: h.ID, Name: h.Name, Category: h.Category, Size: int64(h.Bytes), Downloaded: int64(h.Bytes)}
	switch h.Status {
	case "Completed":
		s.State, s.Progress, s.ContentPath = domain.StateCompleted, 1, c.local(h.Storage)
		if s.ContentPath != "" {
			s.Files = []string{s.ContentPath}
		}
	case "Failed":
		s.State, s.Progress, s.Error = domain.StateFailed, 1, h.Fail
	default: // verifying, repairing, extracting, moving: still working
		s.State, s.Progress = domain.StateDownloading, 1
	}
	return s
}

// Remove deletes a job from the queue and the history, with or without its
// files. Removing an unknown job succeeds.
func (c *Client) Remove(ctx context.Context, id string, deleteFiles bool) error {
	q := url.Values{"name": {"delete"}, "value": {id}, "del_files": {boolNum(deleteFiles)}}
	if err := c.call(ctx, "queue", q, maxSmall, nil); err != nil {
		return err
	}
	return c.call(ctx, "history", q, maxSmall, nil)
}

func boolNum(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// Pause stops one job.
func (c *Client) Pause(ctx context.Context, id string) error {
	return c.call(ctx, "queue", url.Values{"name": {"pause"}, "value": {id}}, maxSmall, nil)
}

// Resume restarts one job.
func (c *Client) Resume(ctx context.Context, id string) error {
	return c.call(ctx, "queue", url.Values{"name": {"resume"}, "value": {id}}, maxSmall, nil)
}

// SetPriority moves a job in the queue. A job no longer queued (finished)
// has no position, which is not an error.
func (c *Client) SetPriority(ctx context.Context, id string, p plugin.Priority) error {
	qi, err := c.queue(ctx, nil)
	if err != nil {
		return err
	}
	idx := slices.IndexFunc(qi.Slots, func(s queueSlot) bool { return s.ID == id })
	if idx < 0 {
		return nil
	}
	pos, ok := position(p, qi.Slots[idx].Index, len(qi.Slots))
	if !ok {
		return plugin.Errorf(plugin.ClassConfig, "unknown priority %q", p)
	}
	return c.call(ctx, "switch", url.Values{"value": {id}, "value2": {strconv.Itoa(pos)}}, maxSmall, nil)
}

func position(p plugin.Priority, cur, n int) (int, bool) {
	switch p {
	case plugin.PrioTop:
		return 0, true
	case plugin.PrioBottom:
		return n - 1, true
	case plugin.PrioUp:
		return max(cur-1, 0), true
	case plugin.PrioDown:
		return min(cur+1, n-1), true
	}
	return 0, false
}

// sabLevels are SABnzbd's job priorities: -1 low, 0 normal, 1 high, 2 force.
var sabLevels = map[plugin.Priority]string{plugin.PrioLow: "-1", plugin.PrioNormal: "0", plugin.PrioHigh: "1", plugin.PrioForce: "2"}

// SetPriorityLevel sets a queued job's priority; SABnzbd reorders its queue
// by it. A finished job has no priority, which is not an error.
func (c *Client) SetPriorityLevel(ctx context.Context, id string, level plugin.Priority) error {
	v, ok := sabLevels[level]
	if !ok {
		return plugin.Errorf(plugin.ClassConfig, "unknown priority level %q", level)
	}
	return c.call(ctx, "queue", url.Values{"name": {"priority"}, "value": {id}, "value2": {v}}, maxSmall, nil)
}

// EnsureCategory creates a missing category through the configuration API;
// an existing one is left as it is, its folder managed in SABnzbd. When the
// configuration is locked the error says to create it by hand.
func (c *Client) EnsureCategory(ctx context.Context, category, savePath string) error {
	if ok, err := c.hasCategory(ctx, category); err != nil || ok {
		return err
	}
	q := url.Values{"section": {"categories"}, "name": {category}}
	if savePath != "" {
		dir, _ := c.paths.ToRemote(savePath)
		q.Set("dir", dir)
	}
	err := c.call(ctx, "set_config", q, maxSmall, nil)
	if err == nil {
		var ok bool
		if ok, err = c.hasCategory(ctx, category); err == nil && !ok {
			err = errors.New("it is still missing")
		}
	}
	if err != nil {
		return plugin.Errorf(plugin.ClassConfig, "create the category %q in SABnzbd (creating it failed: %v)", category, err)
	}
	return nil
}

// hasCategory reports whether SABnzbd knows the category; it compares
// without case, as SABnzbd stores names in lower case.
func (c *Client) hasCategory(ctx context.Context, category string) (bool, error) {
	var out struct {
		Categories []string `json:"categories"`
	}
	if err := c.call(ctx, "get_cats", nil, maxSmall, &out); err != nil {
		return false, err
	}
	return slices.ContainsFunc(out.Categories, func(s string) bool { return strings.EqualFold(s, category) }), nil
}
