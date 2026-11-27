//go:build !unix

package preflight

// SetUmask does nothing where the platform has no file creation mask.
func SetUmask(int) int { return 0 }
