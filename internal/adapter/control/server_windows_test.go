package control

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// foreignOwner is the SID of the Null account, which no process runs as.
const foreignOwner = "S-1-0-0"

var errAddrInUse = windows.WSAEADDRINUSE

func TestDirectoryIsPrivateToTheUser(t *testing.T) {
	srv := startServer(t, nil)
	self, err := currentOwner()
	if err != nil {
		t.Fatal(err)
	}
	want, err := windows.SecurityDescriptorFromString("D:P" + privateACE(self))
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetNamedSecurityInfo(filepath.Dir(srv.Path()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if !isPrivate(sd.String(), want.String()) {
		t.Fatalf("directory DACL = %s, want %s", sd, want)
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\Users\u\AppData\Local`)
	dir, err := DefaultDir()
	if err != nil || dir != `C:\Users\u\AppData\Local\netmon-tray` {
		t.Fatalf("DefaultDir() = %q, %v", dir, err)
	}
	for _, bad := range []string{"", `relative\dir`} {
		t.Setenv("LOCALAPPDATA", bad)
		if _, err := DefaultDir(); err == nil {
			t.Fatalf("DefaultDir() accepted LOCALAPPDATA=%q", bad)
		}
	}
}

func TestCurrentOwnerIsThisProcessUser(t *testing.T) {
	self, err := currentOwner()
	if err != nil {
		t.Fatal(err)
	}
	got, err := processOwner(uint32(os.Getpid()))
	if err != nil || got != self {
		t.Fatalf("processOwner(self) = %q, %v, want %q", got, err, self)
	}
}
