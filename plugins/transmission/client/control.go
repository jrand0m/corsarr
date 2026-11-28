package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
)

// AddRequest asks Transmission to start a payload: torrent bytes, or a
// magnet link or http(s) address that Transmission fetches itself.
type AddRequest struct {
	URL      string // magnet link or http(s) address of a .torrent
	Torrent  []byte // .torrent bytes; preferred over URL when both are set
	Category string
	SavePath string // host view; empty uses the category directory or the default
	Paused   bool
}

type addedTorrent struct {
	HashString string `json:"hashString"`
	Name       string `json:"name"`
}

type addResult struct {
	Added     *addedTorrent `json:"torrent-added"`
	Duplicate *addedTorrent `json:"torrent-duplicate"`
}

// Add starts a payload and returns its info hash. It is idempotent: a
// torrent Transmission already holds is returned without being added again.
// The category becomes a label where the server has labels (set right after
// adding on servers that do not accept labels when adding) and chooses the
// download directory when one is configured for it.
func (c *Client) Add(ctx context.Context, req AddRequest) (string, error) {
	info, err := c.server(ctx)
	if err != nil {
		return "", err
	}
	args, err := c.addArgs(req, info)
	if err != nil {
		return "", err
	}
	var res addResult
	if err := c.call(ctx, "torrent-add", args, &res); err != nil {
		return "", err
	}
	hash, err := res.hash()
	if err != nil {
		return "", err
	}
	c.remember(req.Category, "")
	if res.Added != nil && req.Category != "" && info.RPCVersion == rpcLabels {
		err = c.call(ctx, "torrent-set", map[string]any{"ids": []string{hash}, "labels": []string{req.Category}}, nil)
	}
	return hash, err
}

// hash names the added or already present torrent.
func (r addResult) hash() (string, error) {
	t := r.Added
	if t == nil {
		t = r.Duplicate
	}
	if t == nil || t.HashString == "" {
		return "", errorf(ClassParse, "transmission accepted the torrent but named no info hash")
	}
	return strings.ToLower(t.HashString), nil
}

func (c *Client) addArgs(req AddRequest, info serverInfo) (map[string]any, error) {
	args, err := payloadArgs(req)
	if err != nil {
		return nil, err
	}
	if err := c.categoryArgs(args, req, info); err != nil {
		return nil, err
	}
	if req.Paused || c.addPause {
		args["paused"] = true
	}
	return args, nil
}

func payloadArgs(req AddRequest) (map[string]any, error) {
	u := strings.TrimSpace(req.URL)
	switch {
	case len(req.Torrent) > 0:
		return map[string]any{"metainfo": base64.StdEncoding.EncodeToString(req.Torrent)}, nil
	case strings.HasPrefix(u, "magnet:"), strings.HasPrefix(u, "http://"), strings.HasPrefix(u, "https://"):
		return map[string]any{"filename": u}, nil
	case u != "":
		return nil, errorf(ClassParse, "payload address must be a magnet link or http(s)")
	}
	return nil, errorf(ClassParse, "payload has neither a magnet link, an address nor torrent bytes")
}

// categoryArgs sets the download directory and labels for the category.
func (c *Client) categoryArgs(args map[string]any, req AddRequest, info serverInfo) error {
	if err := checkCategory(req.Category, false); err != nil {
		return err
	}
	dir := c.Remote(req.SavePath)
	if dir == "" {
		dir = c.dirFor(req.Category)
	}
	if dir != "" {
		args["download-dir"] = dir
	}
	switch {
	case req.Category == "":
	case info.RPCVersion >= rpcAddLabels:
		args["labels"] = []string{req.Category}
	case info.RPCVersion < rpcLabels && dir == "":
		return noLabels(req.Category)
	}
	return nil
}

func noLabels(category string) error {
	return errorf(ClassConfig, "this transmission has no labels; set a download directory for category %q", category)
}

// checkCategory rejects names Transmission cannot store as a label.
func checkCategory(category string, required bool) error {
	switch {
	case required && strings.TrimSpace(category) == "":
		return errorf(ClassConfig, "category is empty")
	case strings.Contains(category, ","):
		return errorf(ClassConfig, "category %q must not contain a comma", category)
	}
	return nil
}

// remember records a category and, when given, its download directory
// (Transmission's view).
func (c *Client) remember(category, dir string) {
	if category == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cats[category] = true
	if dir != "" {
		c.dirs[category] = dir
	}
}

func (c *Client) knownCategories() map[string]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]bool, len(c.cats))
	for k := range c.cats {
		out[k] = true
	}
	return out
}

func (c *Client) dirFor(category string) string {
	if category == "" {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dirs[category]
}

// ids builds the arguments that address one torrent.
func ids(id string) (map[string]any, error) {
	h, err := hashID(id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ids": h}, nil
}

// act runs a method on one torrent with optional extra arguments.
func (c *Client) act(ctx context.Context, method, id string, extra map[string]any) error {
	args, err := ids(id)
	if err != nil {
		return err
	}
	for k, v := range extra {
		args[k] = v
	}
	return c.call(ctx, method, args, nil)
}

// Remove deletes a torrent, with or without its data. Removing an unknown
// torrent succeeds, so a repeat after a crash ends in the same state.
func (c *Client) Remove(ctx context.Context, id string, deleteData bool) error {
	return c.act(ctx, "torrent-remove", id, map[string]any{"delete-local-data": deleteData})
}

// Pause stops a torrent.
func (c *Client) Pause(ctx context.Context, id string) error {
	return c.act(ctx, "torrent-stop", id, nil)
}

// Resume starts a torrent, in its turn when the download queue is full.
func (c *Client) Resume(ctx context.Context, id string) error {
	return c.act(ctx, "torrent-start", id, nil)
}

// Recheck verifies a torrent's data again.
func (c *Client) Recheck(ctx context.Context, id string) error {
	return c.act(ctx, "torrent-verify", id, nil)
}

// Reannounce contacts the torrent's trackers now.
func (c *Client) Reannounce(ctx context.Context, id string) error {
	return c.act(ctx, "torrent-reannounce", id, nil)
}

// Priority is a queue move or a priority level; the values match the host's.
type Priority string

const (
	PrioTop    Priority = "top"
	PrioUp     Priority = "up"
	PrioDown   Priority = "down"
	PrioBottom Priority = "bottom"
	PrioLow    Priority = "low"
	PrioNormal Priority = "normal"
	PrioHigh   Priority = "high"
	PrioForce  Priority = "force"
)

var queueMethods = map[Priority]string{
	PrioTop: "queue-move-top", PrioUp: "queue-move-up", PrioDown: "queue-move-down", PrioBottom: "queue-move-bottom",
}

// bandwidthPriorities are Transmission's per-torrent priority levels.
var bandwidthPriorities = map[Priority]int{PrioLow: -1, PrioNormal: 0, PrioHigh: 1}

// SetPriority moves a torrent in Transmission's queue.
func (c *Client) SetPriority(ctx context.Context, id string, p Priority) error {
	method, ok := queueMethods[p]
	if !ok {
		return errorf(ClassConfig, "unknown priority %q", p)
	}
	return c.act(ctx, method, id, nil)
}

// SetPriorityLevel sets a torrent's bandwidth priority (low, normal, high);
// force starts it now, ahead of the queue limits.
func (c *Client) SetPriorityLevel(ctx context.Context, id string, level Priority) error {
	if level == PrioForce {
		return c.act(ctx, "torrent-start-now", id, nil)
	}
	bp, ok := bandwidthPriorities[level]
	if !ok {
		return errorf(ClassConfig, "unknown priority level %q", level)
	}
	return c.act(ctx, "torrent-set", id, map[string]any{"bandwidthPriority": bp})
}

// EnsureCategory makes a category usable; calling it again changes nothing.
// Transmission has no category objects: a save path (host view) becomes the
// category's download directory, and on servers without labels a category
// needs one.
func (c *Client) EnsureCategory(ctx context.Context, category, savePath string) error {
	if err := checkCategory(category, true); err != nil {
		return err
	}
	c.remember(category, c.Remote(savePath))
	labels, err := c.hasLabels(ctx)
	if err != nil || labels || c.dirFor(category) != "" {
		return err
	}
	return noLabels(category)
}

// SetCategory moves a torrent to another category ("" clears it) by
// replacing its category label; labels that are not categories are kept.
// Servers without labels answer ErrUnsupported.
func (c *Client) SetCategory(ctx context.Context, id, category string) error {
	if err := checkCategory(category, false); err != nil {
		return err
	}
	labels, err := c.hasLabels(ctx)
	if err != nil {
		return err
	}
	if !labels {
		return fmt.Errorf("changing the category of a torrent needs labels: %w", ErrUnsupported)
	}
	rt, err := c.one(ctx, id, "labels")
	if err != nil {
		return err
	}
	if err := c.act(ctx, "torrent-set", id, map[string]any{"labels": c.relabel(rt.Labels, category)}); err != nil {
		return err
	}
	c.remember(category, "")
	return nil
}

// relabel drops the labels that are known categories and adds the new one.
func (c *Client) relabel(have []string, category string) []string {
	known := c.knownCategories()
	out := []string{}
	for _, l := range have {
		if !known[l] && l != category {
			out = append(out, l)
		}
	}
	if category != "" {
		out = append(out, category)
	}
	return out
}
