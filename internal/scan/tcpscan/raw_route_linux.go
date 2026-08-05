//go:build linux

package tcpscan

import (
	"fmt"
	"net"
	"os"
)

// resolveRawRoute reads the kernel route table so cross-subnet probes resolve
// the gateway MAC instead of incorrectly ARPing the final target.
func resolveRawRoute(target net.IP, preferredIface string) (rawRoute, error) {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return rawRoute{}, fmt.Errorf("read Linux route table: %w", err)
	}
	defer f.Close()
	routes, err := parseProcRoutes(f)
	if err != nil {
		return rawRoute{}, err
	}
	route, err := selectProcRoute(routes, target, preferredIface)
	if err != nil {
		return rawRoute{}, err
	}
	sourceIP, err := ifaceFirstIPv4(route.iface)
	if err != nil {
		return rawRoute{}, err
	}
	return rawRoute{Iface: route.iface, SourceIP: sourceIP, NextHop: routeNextHop(route, target)}, nil
}
