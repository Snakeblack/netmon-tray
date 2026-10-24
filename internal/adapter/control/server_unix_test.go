//go:build unix

package control

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// foreignOwner is a uid no test process runs as.
var foreignOwner = os.Getuid() + 1

var errAddrInUse = syscall.EADDRINUSE

func TestSocketIsPrivate(t *testing.T) {
	srv := startServer(t, nil)
	sock, err := os.Stat(srv.Path())
	if err != nil {
		t.Fatal(err)
	}
	if sock.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %v, want 0600", sock.Mode().Perm())
	}
	dir, err := os.Stat(filepath.Dir(srv.Path()))
	if err != nil {
		t.Fatal(err)
	}
	if dir.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, want 0700", dir.Mode().Perm())
	}
}

func TestLooseDirectoryIsTightened(t *testing.T) {
	dir := shortDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	srv, err := Listen(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	info, _ := os.Stat(dir)
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, want 0700", info.Mode().Perm())
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	dir, err := DefaultDir()
	if err != nil || dir != "/run/user/1000/netmon-tray" {
		t.Fatalf("DefaultDir() = %q, %v", dir, err)
	}
	for _, bad := range []string{"", "relative/dir"} {
		t.Setenv("XDG_RUNTIME_DIR", bad)
		if _, err := DefaultDir(); err == nil {
			t.Fatalf("DefaultDir() accepted XDG_RUNTIME_DIR=%q", bad)
		}
	}
}

// Only a refused connection proves a socket is a leftover. Any other dial
// failure leaves it alone.
func TestUnreachableSocketIsNotRemoved(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores the socket mode")
	}
	dir := shortDir(t)
	old, err := Listen(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	old.ln.SetUnlinkOnClose(false)
	old.ln.Close()
	old.releaseLock()
	path := old.Path()
	if err := os.Chmod(path, 0); err != nil { // dial now fails with EACCES
		t.Fatal(err)
	}

	if _, err := Listen(dir, nil); err == nil || errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("err = %v, want a failure that is not ErrAlreadyRunning", err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("the socket was removed on an ambiguous dial error: %v", err)
	}
}
