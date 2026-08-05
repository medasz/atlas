//go:build !raw_capture

package tcpscan

import (
	"context"
	"fmt"
	"net"
)

// 默认构建不链接 gopacket/pcap（避免开发机缺 libpcap/Npcap 时无法编译）。
// 此时 raw 模式一律返回 errRawUnavailable，由调用方降级为 connect。
// 生产镜像 / Windows（已装 Npcap SDK）用 -tags raw_capture 构建以启用真实抓包。

func defaultIface(dst net.IP) (string, net.IP, error) {
	return "", nil, fmt.Errorf("raw capture 未在本构建启用（需 -tags raw_capture 及 libpcap/Npcap）")
}

func openHandle(iface string, opts Options) (linkLayer, error) {
	return nil, fmt.Errorf("raw capture 未启用（构建时未加 -tags raw_capture）")
}

func resolveMAC(handle linkLayer, iface string, srcIP, nextHop net.IP) (net.HardwareAddr, error) {
	return nil, fmt.Errorf("ARP 解析不可用（raw_capture 构建外）")
}

func installRstDrop(ctx context.Context, target, iface string) (func(), error) {
	return nil, nil
}
