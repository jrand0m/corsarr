//go:build unix

package preflight

import "syscall"

// SetUmask sets the process file creation mask and returns the previous one.
func SetUmask(mask int) int { return syscall.Umask(mask) }
