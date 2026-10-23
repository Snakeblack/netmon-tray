//go:build linux

package probe

import (
	"strings"
	"testing"
)

const procRoute = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
wlan0	00000000	0101A8C0	0003	0	0	600	00000000	0	0	0
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
eno1	00000000	0201A8C0	0003	0	0	100	00000000	0	0	0
wlan0	0001A8C0	00000000	0001	0	0	600	00FFFFFF	0	0	0
`

func TestParseDefaultRoutePicksLowestMetric(t *testing.T) {
	got, err := parseDefaultRoute(strings.NewReader(procRoute))
	if err != nil {
		t.Fatalf("parseDefaultRoute() error = %v", err)
	}
	if got.iface != "eno1" || got.gateway.String() != "192.168.1.2" {
		t.Fatalf("route = %s via %s, want eno1 via 192.168.1.2", got.iface, got.gateway)
	}
}

func TestParseDefaultRouteDecodesLittleEndianGateway(t *testing.T) {
	in := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\nwlan0\t00000000\t0101A8C0\t0003\t0\t0\t600\n"
	got, err := parseDefaultRoute(strings.NewReader(in))
	if err != nil || got.gateway.String() != "192.168.1.1" {
		t.Fatalf("route = (%v, %v), want gateway 192.168.1.1", got, err)
	}
}

func TestParseDefaultRouteWithoutDefaultFails(t *testing.T) {
	in := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\nwlan0\t0001A8C0\t00000000\t0001\t0\t0\t600\n"
	if _, err := parseDefaultRoute(strings.NewReader(in)); err == nil {
		t.Fatal("parseDefaultRoute() error = nil, want no default route")
	}
}
