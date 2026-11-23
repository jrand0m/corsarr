//go:build windows

package rootfolders

import (
	"os"

	"golang.org/x/sys/windows"
)

// FreeSpace returns the bytes available to the calling user at path (0 when unknown).
func FreeSpace(path string) int64 {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}
	var avail, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &free); err != nil {
		return 0
	}
	return int64(avail)
}

// ProbePath reads a folder's state: a directory, writable by this process
// (a temporary file can be created and removed), and its free space.
func ProbePath(path string) Probe {
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return Probe{}
	}
	return Probe{Dir: true, Writable: canWrite(path), Free: FreeSpace(path)}
}

func canWrite(dir string) bool {
	f, err := os.CreateTemp(dir, ".corsarr-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name) == nil
}
