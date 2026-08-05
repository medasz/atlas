package tcpscan

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

// rawRoute describes the layer-2 path for a raw IPv4 probe. NextHop is the
// address whose MAC must be resolved: the target for a direct route, or a
// gateway for a routed target.
type rawRoute struct {
	Iface    string
	SourceIP net.IP
	NextHop  net.IP
}

type procRoute struct {
	iface       string
	destination net.IP
	gateway     net.IP
	mask        net.IPMask
	prefixLen   int
	metric      uint64
}

// parseProcRoutes parses Linux /proc/net/route. Its IPv4 values are stored as
// little-endian hexadecimal values.
func parseProcRoutes(r io.Reader) ([]procRoute, error) {
	s := bufio.NewScanner(r)
	var routes []procRoute
	line := 0
	for s.Scan() {
		line++
		if line == 1 {
			continue
		}
		fields := strings.Fields(s.Text())
		if len(fields) == 0 {
			continue
		}
		if len(fields) < 8 {
			return nil, fmt.Errorf("invalid /proc/net/route line %d", line)
		}
		flags, err := strconv.ParseUint(fields[3], 16, 32)
		if err != nil {
			return nil, fmt.Errorf("parse route flags on line %d: %w", line, err)
		}
		if flags&0x1 == 0 { // RTF_UP
			continue
		}
		destination, err := parseProcIPv4(fields[1])
		if err != nil {
			return nil, fmt.Errorf("parse route destination on line %d: %w", line, err)
		}
		gateway, err := parseProcIPv4(fields[2])
		if err != nil {
			return nil, fmt.Errorf("parse route gateway on line %d: %w", line, err)
		}
		maskIP, err := parseProcIPv4(fields[7])
		if err != nil {
			return nil, fmt.Errorf("parse route mask on line %d: %w", line, err)
		}
		mask := net.IPMask(maskIP)
		prefixLen, width := mask.Size()
		if width != net.IPv4len*8 || prefixLen < 0 {
			return nil, fmt.Errorf("invalid route mask on line %d", line)
		}
		metric, err := strconv.ParseUint(fields[6], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse route metric on line %d: %w", line, err)
		}
		routes = append(routes, procRoute{fields[0], destination, gateway, mask, prefixLen, metric})
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("read /proc/net/route: %w", err)
	}
	return routes, nil
}

func parseProcIPv4(value string) (net.IP, error) {
	n, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return nil, err
	}
	var bytes [net.IPv4len]byte
	binary.LittleEndian.PutUint32(bytes[:], uint32(n))
	return net.IPv4(bytes[0], bytes[1], bytes[2], bytes[3]).To4(), nil
}

// selectProcRoute uses longest-prefix matching, breaking ties on the metric.
// A configured interface restricts the routes that may be selected.
func selectProcRoute(routes []procRoute, target net.IP, preferredIface string) (procRoute, error) {
	target = target.To4()
	if target == nil {
		return procRoute{}, fmt.Errorf("raw scan only supports IPv4 routing")
	}
	var selected procRoute
	found := false
	for _, route := range routes {
		if preferredIface != "" && route.iface != preferredIface {
			continue
		}
		if !routeMatches(route, target) {
			continue
		}
		if !found || route.prefixLen > selected.prefixLen ||
			(route.prefixLen == selected.prefixLen && route.metric < selected.metric) {
			selected, found = route, true
		}
	}
	if !found {
		if preferredIface != "" {
			return procRoute{}, fmt.Errorf("interface %s has no route to %s", preferredIface, target)
		}
		return procRoute{}, fmt.Errorf("no route to %s", target)
	}
	return selected, nil
}

func routeMatches(route procRoute, target net.IP) bool {
	for i := 0; i < net.IPv4len; i++ {
		if target[i]&route.mask[i] != route.destination[i]&route.mask[i] {
			return false
		}
	}
	return true
}

func routeNextHop(route procRoute, target net.IP) net.IP {
	if route.gateway == nil || route.gateway.Equal(net.IPv4zero) {
		return target.To4()
	}
	return route.gateway.To4()
}

func ifaceFirstIPv4(name string) (net.IP, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, fmt.Errorf("get interface %s: %w", name, err)
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("list addresses for interface %s: %w", name, err)
	}
	for _, address := range addresses {
		ipNet, ok := address.(*net.IPNet)
		if !ok {
			continue
		}
		if ip := ipNet.IP.To4(); ip != nil && !ip.IsLoopback() {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("interface %s has no usable IPv4 address", name)
}
