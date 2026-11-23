//go:build windows

package store

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes a non-blocking exclusive LockFileEx on the first byte; busy
// reports another holder.
func lockFile(f *os.File) (busy bool, err error) {
	ol := new(windows.Overlapped)
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return true, nil
	}
	return false, err
}

func unlockFile(f *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
}
