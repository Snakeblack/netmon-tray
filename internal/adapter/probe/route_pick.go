package probe

import (
	"errors"
	"net"
)

// routeCandidate is one default route as Windows lists it: the interface
// index, the gateway and the metric that orders it, route plus interface.
type routeCandidate struct {
	index   uint32
	gateway net.IP
	metric  uint32
}

// pickRoute returns the default route Windows uses: the lowest metric, and on
// a tie the first listed.
func pickRoute(candidates []routeCandidate) (routeCandidate, error) {
	if len(candidates) == 0 {
		return routeCandidate{}, errors.New("no default route")
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.metric < best.metric {
			best = c
		}
	}
	return best, nil
}
