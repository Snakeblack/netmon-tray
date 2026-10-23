//go:build !linux

package probe

import (
	"errors"
	"net"
)

type route struct {
	iface   string
	gateway net.IP
}

func (r route) linkUp() bool { return false }

// defaultRoute is implemented per OS; Windows arrives in M6.
func defaultRoute() (route, error) {
	return route{}, errors.New("default route lookup is not implemented on this OS")
}
