package filelock

import (
	"errors"
	"path/filepath"
	"testing"
)

// Two engines in one process still open the file twice, as two processes do.
func TestSecondLockIsRefusedUntilTheFirstCloses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	first, err := TryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := TryLock(path); !errors.Is(err, ErrLocked) {
		if second != nil {
			second.Close()
		}
		t.Fatalf("second TryLock() error = %v, want ErrLocked", err)
	}
	first.Close()

	again, err := TryLock(path)
	if err != nil {
		t.Fatalf("TryLock() after the first closed: %v", err)
	}
	again.Close()
}

func TestMissingDirectoryIsAnError(t *testing.T) {
	_, err := TryLock(filepath.Join(t.TempDir(), "missing", "x.lock"))
	if err == nil || errors.Is(err, ErrLocked) {
		t.Fatalf("TryLock() error = %v, want a plain failure", err)
	}
}
