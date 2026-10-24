package probe

import (
	"context"
	"encoding/binary"
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows answers ICMP echo without privileges through iphlpapi, as ping.exe
// does; raw sockets would need an administrator.
var (
	iphlpapi            = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreateFile  = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpCloseHandle = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho    = iphlpapi.NewProc("IcmpSendEcho")
)

// icmpReplyRoom holds an ICMP_ECHO_REPLY, the echoed data and the 8 bytes of
// an ICMP error the API may write.
const icmpReplyRoom = 64 + 8

func icmpOpen() (windows.Handle, error) {
	if err := procIcmpCreateFile.Find(); err != nil {
		return windows.InvalidHandle, err
	}
	h, _, err := procIcmpCreateFile.Call()
	if windows.Handle(h) == windows.InvalidHandle {
		return windows.InvalidHandle, err
	}
	return windows.Handle(h), nil
}

func icmpClose(h windows.Handle) { procIcmpCloseHandle.Call(uintptr(h)) }

// checkICMP opens and closes an ICMP handle.
func checkICMP() error {
	h, err := icmpOpen()
	if err != nil {
		return err
	}
	icmpClose(h)
	return nil
}

// ping sends one ICMP echo with IcmpSendEcho and waits for the reply.
func ping(ctx context.Context, addr string, timeout time.Duration) bool {
	ip := net.ParseIP(addr).To4()
	if ip == nil {
		return false
	}
	if d, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(d))
	}
	if timeout <= 0 {
		return false
	}
	h, err := icmpOpen()
	if err != nil {
		return false
	}
	defer icmpClose(h)

	data := []byte("netmon")
	reply := make([]byte, icmpReplyRoom+len(data))
	// IPAddr is the address in network order; read its bytes as they lie.
	dest := binary.LittleEndian.Uint32(ip)
	n, _, _ := procIcmpSendEcho.Call(uintptr(h), uintptr(dest),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0,
		uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)),
		uintptr(max(timeout.Milliseconds(), 1)))
	if n == 0 {
		return false
	}
	// ICMP_ECHO_REPLY starts with Address and Status; IP_SUCCESS is 0.
	return binary.LittleEndian.Uint32(reply[4:8]) == 0
}
