//go:build !linux

package tcpscan

import (
	"fmt"
	"net"
)

// Non-Linux builds lack /proc/net/route, so only directly connected routes are
// used. A routed raw scan fails closed and the caller falls back to connect.
func resolveRawRoute(target net.IP, preferredIface string) (rawRoute, error) {
	target = target.To4()
	if target == nil {
		return rawRoute{}, fmt.Errorf("raw scan only supports IPv4 routing")
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return rawRoute{}, fmt.Errorf("list network interfaces: %w", err)
	}
	for _, iface := range interfaces {
		if preferredIface != "" && iface.Name != preferredIface {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ipNet, ok := address.(*net.IPNet)
			if ok && ipNet.IP.To4() != nil && ipNet.Contains(target) {
				return rawRoute{Iface: iface.Name, SourceIP: ipNet.IP.To4(), NextHop: target}, nil
			}
		}
	}
	if preferredIface != "" {
		return rawRoute{}, fmt.Errorf("interface %s has no direct route to %s", preferredIface, target)
	}
	return rawRoute{}, fmt.Errorf("routed raw scans require Linux route discovery")
}
