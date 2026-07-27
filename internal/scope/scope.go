package scope

import (
	"fmt"
	"net"
	"strings"
)

// maxExpand 单次 CIDR 展开上限，避免超大网段撑爆任务表
const maxExpand = 1 << 16

// Expand 将任务 scope 展开为待扫描目标列表，支持 ip / cidr / domain
func Expand(scope map[string]any) ([]string, error) {
	raw, ok := scope["targets"]
	if !ok {
		return nil, fmt.Errorf("scope.targets required")
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("scope.targets must be a list")
	}
	out := []string{}
	seen := map[string]bool{}
	for _, item := range list {
		s, _ := item.(string)
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		for _, t := range expandOne(s) {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out, nil
}

func expandOne(s string) []string {
	if _, ipNet, err := net.ParseCIDR(s); err == nil {
		ips, err := expandCIDR(ipNet)
		if err != nil {
			return []string{s}
		}
		return ips
	}
	// 域名（含端口后缀如 example.com:8443）按原值保留
	if net.ParseIP(s) == nil && !strings.Contains(s, "/") {
		return []string{s}
	}
	// 单个 IP
	if net.ParseIP(s) != nil {
		return []string{s}
	}
	return []string{s}
}

func expandCIDR(ipNet *net.IPNet) ([]string, error) {
	var ips []string
	for ip := ipNet.IP.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip) {
		addr := ip.String()
		if isNetworkOrBroadcast(ipNet, ip) {
			continue
		}
		ips = append(ips, addr)
		if len(ips) > maxExpand {
			return nil, fmt.Errorf("cidr %s exceeds expand limit %d", ipNet.String(), maxExpand)
		}
	}
	return ips, nil
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func isNetworkOrBroadcast(ipNet *net.IPNet, ip net.IP) bool {
	if ip.Equal(ipNet.IP) {
		return true
	}
	broadcast := make(net.IP, len(ip))
	for i := range ip {
		broadcast[i] = ip[i] | ^ipNet.Mask[i]
	}
	return ip.Equal(broadcast)
}
