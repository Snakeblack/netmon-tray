// Package filelock takes the exclusive lock that makes one engine the owner
// of a resource (ADR 0003). The system drops it when the process exits, so a
// crashed engine leaves no stale lock behind.
package filelock

import (
	"errors"
	"os"
)

// ErrLocked means another open file already holds the lock.
var ErrLocked = errors.New("lock held by another process")

// TryLock creates path with mode 0600 if needed and locks it without waiting.
// The lock lasts until the returned file is closed.
func TryLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lock(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
