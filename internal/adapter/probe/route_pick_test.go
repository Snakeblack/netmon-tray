package probe

import (
	"net"
	"testing"
)

func TestPickRouteTakesTheLowestMetric(t *testing.T) {
	got, err := pickRoute([]routeCandidate{
		{index: 12, gateway: net.IPv4(192, 168, 1, 1), metric: 35}, // wifi
		{index: 7, gateway: net.IPv4(192, 168, 1, 2), metric: 25},  // ethernet
		{index: 20, gateway: net.IPv4(10, 8, 0, 1), metric: 5256},  // a VPN kept as fallback
	})
	if err != nil || got.index != 7 || !got.gateway.Equal(net.IPv4(192, 168, 1, 2)) {
		t.Fatalf("pickRoute() = (%+v, %v), want interface 7 via 192.168.1.2", got, err)
	}
}

func TestPickRouteKeepsTheFirstOnATie(t *testing.T) {
	got, err := pickRoute([]routeCandidate{
		{index: 3, gateway: net.IPv4(192, 168, 0, 1), metric: 50},
		{index: 4, gateway: net.IPv4(192, 168, 0, 2), metric: 50},
	})
	if err != nil || got.index != 3 {
		t.Fatalf("pickRoute() = (%+v, %v), want interface 3", got, err)
	}
}

func TestPickRouteWithoutCandidatesFails(t *testing.T) {
	if _, err := pickRoute(nil); err == nil {
		t.Fatal("pickRoute(nil) error = nil, want no default route")
	}
}
