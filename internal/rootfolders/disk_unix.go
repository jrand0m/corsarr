//go:build !windows

package rootfolders

import (
	"os"
	"syscall"
)

// FreeSpace returns the bytes available to unprivileged users at path (0 when unknown).
func FreeSpace(path string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0
	}
	return int64(st.Bavail) * int64(st.Bsize) //nolint:unconvert // field types differ between linux and darwin
}

// writeOK is the W_OK mode of access(2).
const writeOK = 0x2

// ProbePath reads a folder's state: a directory, writable by this process
// (access(2), which also sees read-only mounts), and its free space.
func ProbePath(path string) Probe {
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return Probe{}
	}
	return Probe{Dir: true, Writable: syscall.Access(path, writeOK) == nil, Free: FreeSpace(path)}
}
