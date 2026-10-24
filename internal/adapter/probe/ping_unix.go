//go:build !windows

package probe

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// checkICMP opens and closes an unprivileged ping socket.
func checkICMP() error {
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return fmt.Errorf("check net.ipv4.ping_group_range: %w", err)
	}
	return conn.Close()
}

// ping sends one ICMP echo through an unprivileged datagram socket and waits
// for the reply.
func ping(ctx context.Context, addr string, timeout time.Duration) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return false
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)

	seq := os.Getpid() & 0xffff
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Body: &icmp.Echo{ID: seq, Seq: seq, Data: []byte("netmon")},
	}
	wire, err := msg.Marshal(nil)
	if err != nil {
		return false
	}
	if _, err := conn.WriteTo(wire, &net.UDPAddr{IP: ip}); err != nil {
		return false
	}

	buf := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return false
		}
		reply, err := icmp.ParseMessage(1, buf[:n])
		if err != nil {
			continue
		}
		if reply.Type == ipv4.ICMPTypeEchoReply {
			return true
		}
	}
}
