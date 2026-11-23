//go:build !windows

package api

import "syscall"

// rusageRSS is peak RSS; darwin reports bytes, other systems kilobytes.
func rusageRSS(goos string) int64 {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru) // cannot fail for RUSAGE_SELF
	if goos == "darwin" {
		return ru.Maxrss
	}
	return ru.Maxrss * 1024
}
