//go:build unix

package storage

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// lockHistory takes the owner lock of the history at path without waiting.
// The kernel drops it when the process exits, so a crashed engine leaves no
// stale lock behind.
func lockHistory(path string) (*os.File, error) {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create history lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrInUse
		}
		return nil, fmt.Errorf("lock history: %w", err)
	}
	return f, nil
}
