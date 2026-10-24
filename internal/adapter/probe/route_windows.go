package probe

import (
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

// route is the default route: the interface that carries it and its gateway.
type route struct {
	index   uint32
	gateway net.IP
}

// linkUp reports whether the interface of the default route is up.
func (r route) linkUp() bool {
	row := windows.MibIfRow2{InterfaceIndex: r.index}
	if err := windows.GetIfEntry2Ex(windows.MibIfTableNormalWithoutStatistics, &row); err != nil {
		return false
	}
	return row.OperStatus == windows.IfOperStatusUp
}

// defaultRoute lists the IPv4 default routes and picks the one Windows uses.
func defaultRoute() (route, error) {
	var table *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &table); err != nil {
		return route{}, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))

	var candidates []routeCandidate
	for _, row := range table.Rows() {
		if row.DestinationPrefix.PrefixLength != 0 || row.NextHop.Family != windows.AF_INET {
			continue
		}
		// For AF_INET, SOCKADDR_INET holds a sockaddr_in: the address
		// follows family and port, in network order.
		addr := (*[4]byte)(unsafe.Pointer(&row.NextHop.Data[0]))
		candidates = append(candidates, routeCandidate{
			index:   row.InterfaceIndex,
			gateway: net.IPv4(addr[0], addr[1], addr[2], addr[3]),
			metric:  row.Metric + interfaceMetric(row.InterfaceLuid),
		})
	}
	best, err := pickRoute(candidates)
	if err != nil {
		return route{}, err
	}
	return route{index: best.index, gateway: best.gateway}, nil
}

// interfaceMetric is the IPv4 metric of the interface, which Windows adds to
// the route's; 0 when it cannot be read.
func interfaceMetric(luid uint64) uint32 {
	row := windows.MibIpInterfaceRow{Family: windows.AF_INET, InterfaceLuid: luid}
	if err := windows.GetIpInterfaceEntry(&row); err != nil {
		return 0
	}
	return row.Metric
}
