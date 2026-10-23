package probe

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

// route is the default route: the interface that carries it and its gateway.
type route struct {
	iface   string
	gateway net.IP
}

// linkUp reports whether the interface of the default route is up.
func (r route) linkUp() bool {
	b, err := os.ReadFile("/sys/class/net/" + r.iface + "/operstate")
	if err != nil {
		return false
	}
	state := strings.TrimSpace(string(b))
	return state == "up" || state == "unknown"
}

func defaultRoute() (route, error) {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return route{}, err
	}
	defer f.Close()
	return parseDefaultRoute(f)
}

// parseDefaultRoute reads /proc/net/route and returns the default route with
// the lowest metric.
func parseDefaultRoute(r io.Reader) (route, error) {
	var (
		best   route
		metric = -1
	)
	sc := bufio.NewScanner(r)
	sc.Scan() // header
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 7 || f[1] != "00000000" {
			continue
		}
		gw, err := parseHexIPv4(f[2])
		if err != nil {
			continue
		}
		m, err := strconv.Atoi(f[6])
		if err != nil {
			continue
		}
		if metric == -1 || m < metric {
			best, metric = route{iface: f[0], gateway: gw}, m
		}
	}
	if err := sc.Err(); err != nil {
		return route{}, err
	}
	if metric == -1 {
		return route{}, fmt.Errorf("no default route")
	}
	return best, nil
}

// parseHexIPv4 decodes the little-endian hex address of /proc/net/route.
func parseHexIPv4(s string) (net.IP, error) {
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return nil, err
	}
	return net.IPv4(byte(v), byte(v>>8), byte(v>>16), byte(v>>24)), nil
}
