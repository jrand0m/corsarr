package client

import (
	"context"
	"errors"
	"strings"
)

func id(hash string) string { return strings.ToUpper(hash) }

// eraseEvent is the event whose handlers run when a download is erased; web
// front ends hook a data-erasing handler onto it that deletes the payload of
// downloads flagged in custom5.
const eraseEvent = "event.download.erased"

// SupportsEraseData reports whether removing a download can delete its data:
// rTorrent never deletes data itself, so this needs the data-erasing handler
// a web front end installs. The answer is cached once known.
func (c *Client) SupportsEraseData(ctx context.Context) (bool, error) {
	c.mu.Lock()
	known := c.eraseKey
	c.mu.Unlock()
	if known != nil {
		return *known, nil
	}
	v, err := c.Call(ctx, "method.get", "", eraseEvent)
	if err != nil && !errors.As(err, new(*Fault)) {
		return false, err
	}
	ok := hasEraseHandler(v) // a fault means no such event or no introspection
	c.mu.Lock()
	c.eraseKey = &ok
	c.mu.Unlock()
	return ok, nil
}

func hasEraseHandler(v any) bool {
	handlers, _ := v.(map[string]any)
	for key := range handlers {
		if strings.Contains(strings.ToLower(key), "erasedata") {
			return true
		}
	}
	return false
}

// Remove erases a download, deleting its data too when deleteData is set.
// Removing an unknown hash succeeds, so a repeat ends in the same state.
// Deleting data without a data-erasing handler fails with an
// ErrUnsupported error before anything is removed.
func (c *Client) Remove(ctx context.Context, hash string, deleteData bool) error {
	have, err := c.Exists(ctx, hash)
	if err != nil || !have {
		return err
	}
	reqs := []Request{{Method: "d.erase", Params: []any{id(hash)}}}
	if deleteData {
		ok, err := c.SupportsEraseData(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return errorf(KindUnsupported, "removing a download with its data needs the data-erasing handler of a web front end")
		}
		reqs = append([]Request{{Method: "d.custom5.set", Params: []any{id(hash), "1"}}}, reqs...)
	}
	_, err = c.Batch(ctx, reqs)
	return err
}

// flags reads d.state and d.is_active of one download.
func (c *Client) flags(ctx context.Context, hash string) (started, active bool, err error) {
	v, err := c.Batch(ctx, []Request{
		{Method: "d.state", Params: []any{id(hash)}},
		{Method: "d.is_active", Params: []any{id(hash)}},
	})
	if err != nil {
		return false, false, err
	}
	return num(v[0]) != 0, num(v[1]) != 0, nil
}

// Pause pauses a running download (d.pause); it stays started, so Resume
// picks it up again. A stopped or already paused download is left alone.
func (c *Client) Pause(ctx context.Context, hash string) error {
	started, active, err := c.flags(ctx, hash)
	if err != nil || !started || !active {
		return err
	}
	_, err = c.Call(ctx, "d.pause", id(hash))
	return err
}

// Resume runs a download again: d.start when it is stopped, d.resume when
// it is paused. A running download is left alone.
func (c *Client) Resume(ctx context.Context, hash string) error {
	started, active, err := c.flags(ctx, hash)
	switch {
	case err != nil:
		return err
	case !started:
		return c.Start(ctx, hash)
	case !active:
		_, err = c.Call(ctx, "d.resume", id(hash))
	}
	return err
}

// Stop stops a download (d.stop).
func (c *Client) Stop(ctx context.Context, hash string) error {
	_, err := c.Call(ctx, "d.stop", id(hash))
	return err
}

// Start starts a download (d.start).
func (c *Client) Start(ctx context.Context, hash string) error {
	_, err := c.Call(ctx, "d.start", id(hash))
	return err
}

// Priority is rTorrent's download priority.
type Priority int

const (
	PriorityOff    Priority = 0 // nothing is fetched
	PriorityLow    Priority = 1
	PriorityNormal Priority = 2
	PriorityHigh   Priority = 3
)

var levels = map[string]Priority{
	"off": PriorityOff, "low": PriorityLow, "normal": PriorityNormal, "high": PriorityHigh,
	// rTorrent has no queue to jump; force is the highest priority.
	"force": PriorityHigh,
}

// Level returns the priority for a level name: off, low, normal, high or
// force (the same as high).
func Level(name string) (Priority, error) {
	p, ok := levels[name]
	if !ok {
		return 0, errorf(KindConfig, "unknown priority level %q", name)
	}
	return p, nil
}

// SetPriority sets a download's priority (d.priority.set) and applies it.
func (c *Client) SetPriority(ctx context.Context, hash string, p Priority) error {
	if p < PriorityOff || p > PriorityHigh {
		return errorf(KindConfig, "priority must be 0 to 3, not %d", p)
	}
	_, err := c.Batch(ctx, []Request{
		{Method: "d.priority.set", Params: []any{id(hash), int64(p)}},
		{Method: "d.update_priorities", Params: []any{id(hash)}},
	})
	return err
}

// Move shifts a download's priority as a queue move: top is high, bottom is
// low, up and down step one level within low..high. rTorrent has no queue
// order, so priority is the nearest equivalent.
func (c *Client) Move(ctx context.Context, hash, move string) error {
	switch move {
	case "top":
		return c.SetPriority(ctx, hash, PriorityHigh)
	case "bottom":
		return c.SetPriority(ctx, hash, PriorityLow)
	case "up", "down":
		return c.step(ctx, hash, map[string]Priority{"up": 1, "down": -1}[move])
	}
	return errorf(KindConfig, "unknown priority move %q", move)
}

func (c *Client) step(ctx context.Context, hash string, delta Priority) error {
	v, err := c.Call(ctx, "d.priority", id(hash))
	if err != nil {
		return err
	}
	p := min(max(Priority(num(v))+delta, PriorityLow), PriorityHigh)
	return c.SetPriority(ctx, hash, p)
}

// SetLabel sets a download's label (its category) in the configured custom
// field; "" clears it.
func (c *Client) SetLabel(ctx context.Context, hash, label string) error {
	_, err := c.Call(ctx, "d."+c.cfg.LabelField+".set", id(hash), encodeLabel(label))
	return err
}

// EnsureLabel exists for symmetry with clients that keep a category list:
// rTorrent labels are free-form, so there is nothing to create.
func (c *Client) EnsureLabel(context.Context, string) error { return nil }

// CheckHash verifies a download's data again (d.check_hash).
func (c *Client) CheckHash(ctx context.Context, hash string) error {
	_, err := c.Call(ctx, "d.check_hash", id(hash))
	return err
}

// Reannounce contacts the download's trackers now. A build without a
// manual announce command answers with an ErrUnsupported error.
func (c *Client) Reannounce(ctx context.Context, hash string) error {
	_, err := c.callFirst(ctx, []string{"d.tracker_announce.force", "d.tracker_announce"}, id(hash))
	return err
}
