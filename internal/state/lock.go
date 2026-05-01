package state

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Lock acquires an exclusive advisory lock on ~/.local/state/mise-en-place/state.lock.
// The kernel releases it automatically when the returned file is closed
// (including on process death). Callers must call Release to unlock + close.
func Lock() (*os.File, error) {
	path, err := lockPath()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquire flock: %w", err)
	}
	return f, nil
}

// Release unlocks and closes a lock file previously returned by Lock.
func Release(f *os.File) error {
	if f == nil {
		return nil
	}
	// Closing the fd releases the flock, but we unlock explicitly so the
	// intent is visible.
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
	return f.Close()
}
