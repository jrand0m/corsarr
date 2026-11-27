package sabnzbd

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/jrand0m/corsarr/internal/plugin"
)

var (
	_ plugin.GlobalPauser = (*Client)(nil)
	_ plugin.SpeedLimiter = (*Client)(nil)
	_ plugin.ClientStats  = (*Client)(nil)
)

// PauseAll pauses the whole download queue.
func (c *Client) PauseAll(ctx context.Context) error { return c.call(ctx, "pause", nil, maxSmall, nil) }

// ResumeAll resumes the whole download queue.
func (c *Client) ResumeAll(ctx context.Context) error {
	return c.call(ctx, "resume", nil, maxSmall, nil)
}

// SpeedLimits reads the download limit; usenet has no upload limit.
func (c *Client) SpeedLimits(ctx context.Context) (plugin.SpeedLimits, error) {
	qi, err := c.queue(ctx, url.Values{"limit": {"1"}})
	return plugin.SpeedLimits{DownKiB: int64(qi.SpeedLimitAbs) / 1024}, err
}

// SetSpeedLimits sets the download limit (0 = unlimited). An upload limit is
// not supported.
func (c *Client) SetSpeedLimits(ctx context.Context, l plugin.SpeedLimits) error {
	if l.UpKiB != 0 {
		return fmt.Errorf("upload limit: %w", plugin.ErrUnsupported)
	}
	if l.DownKiB < 0 {
		return plugin.Errorf(plugin.ClassConfig, "speed limits must be 0 (unlimited) or more")
	}
	value := "100" // percent of the line speed: no limit
	if l.DownKiB > 0 {
		value = strconv.FormatInt(l.DownKiB, 10) + "K"
	}
	return c.call(ctx, "config", url.Values{"name": {"speedlimit"}, "value": {value}}, maxSmall, nil)
}

// AltSpeed is not a SABnzbd concept.
func (c *Client) AltSpeed(context.Context) (bool, error) {
	return false, fmt.Errorf("alternative speed mode: %w", plugin.ErrUnsupported)
}

// SetAltSpeed is not a SABnzbd concept.
func (c *Client) SetAltSpeed(context.Context, bool) error {
	return fmt.Errorf("alternative speed mode: %w", plugin.ErrUnsupported)
}

// Stats reads speed, free space, the paused flag and the limit from one
// queue call.
func (c *Client) Stats(ctx context.Context) (plugin.ClientStatus, error) {
	qi, err := c.queue(ctx, url.Values{"limit": {"1"}})
	if err != nil {
		return plugin.ClientStatus{}, err
	}
	return plugin.ClientStatus{DownSpeed: int64(qi.KBPerSec * 1024), FreeSpace: int64(qi.DiskSpace * (1 << 30)),
		Paused: qi.Paused, Limits: plugin.SpeedLimits{DownKiB: int64(qi.SpeedLimitAbs) / 1024}}, nil
}
