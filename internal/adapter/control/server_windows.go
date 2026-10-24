package control

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// owner identifies a user: the string form of its SID.
type owner = string

// sioAFUnixGetPeerPID is SIO_AF_UNIX_GETPEERPID of afunix.h,
// _WSAIOR(IOC_VENDOR, 256): the process id at the other end of the socket.
const sioAFUnixGetPeerPID = 0x58000100

// DefaultDir is %LOCALAPPDATA%\netmon-tray.
func DefaultDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", errors.New("LOCALAPPDATA no está definido")
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("LOCALAPPDATA %q no es una ruta absoluta", base)
	}
	return filepath.Join(base, "netmon-tray"), nil
}

// privateDir makes dir a directory whose protected ACL grants access to the
// current user alone; what it holds inherits that ACL. It rewrites the ACL
// only when it differs, since the rewrite walks every file below.
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
	self, err := currentOwner()
	if err != nil {
		return err
	}
	want, err := windows.SecurityDescriptorFromString("D:P" + privateACE(self))
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err == nil && isPrivate(sd.String(), want.String()) {
		return nil
	}
	dacl, _, err := want.DACL()
	if err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("%s: no se pudo restringir al usuario: %w", dir, err)
	}
	return nil
}

// privateSocket does nothing: the socket inherits the ACL of its directory.
func privateSocket(string) error { return nil }

func currentOwner() (owner, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

// peerOwner is the user of the process at the other end. A process of
// another user cannot be opened, which also rejects it.
func peerOwner(conn *net.UnixConn) (owner, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return "", err
	}
	var pid uint32
	var ioErr error
	err = raw.Control(func(fd uintptr) {
		var n uint32
		ioErr = windows.WSAIoctl(windows.Handle(fd), sioAFUnixGetPeerPID, nil, 0,
			(*byte)(unsafe.Pointer(&pid)), uint32(unsafe.Sizeof(pid)), &n, nil, 0)
	})
	if err == nil {
		err = ioErr
	}
	if err != nil {
		return "", fmt.Errorf("pid del cliente: %w", err)
	}
	return processOwner(pid)
}

func processOwner(pid uint32) (owner, error) {
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", fmt.Errorf("proceso %d: %w", pid, err)
	}
	defer windows.CloseHandle(p)
	var token windows.Token
	if err := windows.OpenProcessToken(p, windows.TOKEN_QUERY, &token); err != nil {
		return "", fmt.Errorf("token del proceso %d: %w", pid, err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func isAddrInUse(err error) bool { return errors.Is(err, windows.WSAEADDRINUSE) }

func isRefused(err error) bool { return errors.Is(err, windows.WSAECONNREFUSED) }
