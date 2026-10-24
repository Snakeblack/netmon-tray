//go:build unix

package control

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// owner identifies a user: the uid.
type owner = int

// DefaultDir is $XDG_RUNTIME_DIR/netmon-tray.
func DefaultDir() (string, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		return "", errors.New("XDG_RUNTIME_DIR no está definido")
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("XDG_RUNTIME_DIR %q no es una ruta absoluta", base)
	}
	return filepath.Join(base, "netmon-tray"), nil
}

// privateDir makes dir a directory only its owner, the current user, can
// enter.
func privateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s no es un directorio", dir)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s pertenece a otro usuario", dir)
	}
	if info.Mode().Perm() != 0o700 {
		return os.Chmod(dir, 0o700)
	}
	return nil
}

// privateSocket leaves the socket at path to its owner alone.
func privateSocket(path string) error {
	return os.Chmod(path, 0o600)
}

func currentOwner() (owner, error) {
	return os.Getuid(), nil
}

// peerOwner is the uid of the process at the other end, from SO_PEERCRED.
func peerOwner(conn *net.UnixConn) (owner, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return -1, err
	}
	uid := -1
	var credErr error
	err = raw.Control(func(fd uintptr) {
		var cred *syscall.Ucred
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if credErr == nil {
			uid = int(cred.Uid)
		}
	})
	if err == nil {
		err = credErr
	}
	return uid, err
}

func isAddrInUse(err error) bool { return errors.Is(err, syscall.EADDRINUSE) }

func isRefused(err error) bool { return errors.Is(err, syscall.ECONNREFUSED) }
