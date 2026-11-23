//go:build !windows

package store

import (
	"errors"
	"os"
	"syscall"
)

// lockFile takes a non-blocking exclusive flock; busy reports another holder.
func lockFile(f *os.File) (busy bool, err error) {
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return true, nil
	}
	return false, err
}

func unlockFile(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
