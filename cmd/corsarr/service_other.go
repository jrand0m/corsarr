//go:build !windows

package main

import (
	"context"
	"fmt"
)

// service is Windows-only; systemd and launchd units run corsarr elsewhere.
func (c cli) service(_ context.Context, action string) int {
	if action == "" {
		return serviceUsage(c.stderr)
	}
	fmt.Fprintln(c.stderr, "service mode is only available on Windows; use the systemd or launchd samples in deploy/")
	return 2
}
