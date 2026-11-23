package store

import (
	"fmt"
	"os"
	"strconv"
)

// acquireLock takes an exclusive advisory lock on path; a second corsarr on
// the same data dir is refused. The OS drops the lock if the process dies,
// so a stale file never blocks a restart.
func acquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("data dir is not writable: %w", err)
	}
	busy, err := lockFile(f)
	if busy || err != nil {
		f.Close()
		if busy {
			return nil, fmt.Errorf("another corsarr process is using %s", path)
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	return f, nil
}

func releaseLock(f *os.File) {
	unlockFile(f)
	_ = f.Close()
}
