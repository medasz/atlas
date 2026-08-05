//go:build raw_capture

package tcpscan

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// 此文件在 -tags raw_capture 构建下启用，链接 gopacket/pcap：
// Linux 需 libpcap-dev；Windows 需 Npcap SDK（含 wpcap.lib / Packet.lib）。

const pcapReadTimeout = 200 * time.Millisecond

// pcapLink 适配 *pcap.Handle 以满足 linkLayer 接口。
// pcap.Handle.LinkType() itself implements gopacket.Decoder. Keep that
// decoder intact: converting a DLT to a LayerType loses its Decode method.
type pcapLink struct{ *pcap.Handle }

func (p pcapLink) LinkType() gopacket.Decoder { return p.Handle.LinkType() }

func defaultIface(dst net.IP) (string, net.IP, error) {
	devs, err := pcap.FindAllDevs()
	if err != nil {
		return "", nil, fmt.Errorf("枚举网卡失败: %w", err)
	}
	for _, d := range devs {
		if d.Name == "lo" || d.Name == "lo0" {
			continue
		}
		for _, addr := range d.Addresses {
			if ip := addr.IP.To4(); ip != nil && !ip.IsLoopback() {
				return d.Name, ip, nil
			}
		}
	}
	return "", nil, fmt.Errorf("未找到可用抓包网卡（目标 %s）", dst)
}

func openHandle(iface string, opts Options) (linkLayer, error) {
	handle, err := pcap.OpenLive(iface, 65536, true, pcapReadTimeout)
	if err != nil {
		return nil, fmt.Errorf("打开抓包句柄 %s 失败: %w", iface, err)
	}
	if os.Getenv("ATLAS_RAW_DEBUG") == "1" {
		fmt.Printf("tcpscan: raw debug opened iface=%s pcap_link_type=%v decoder=%T\n", iface, handle.LinkType(), handle.LinkType())
	}
	return pcapLink{handle}, nil
}

// resolveMAC resolves the layer-2 next-hop MAC. The next hop is the target on
// a directly connected route, or the gateway for a routed target.
func resolveMAC(handle linkLayer, iface string, srcIP, nextHop net.IP) (net.HardwareAddr, error) {
	ifaceObj, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, fmt.Errorf("获取网卡 %s 信息失败: %w", iface, err)
	}
	src := ifaceObj.HardwareAddr

	eth := &layers.Ethernet{
		SrcMAC:       src,
		DstMAC:       net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		EthernetType: layers.EthernetTypeARP,
	}
	arp := &layers.ARP{
		AddrType:          layers.LinkTypeEthernet,
		Protocol:          layers.EthernetTypeIPv4,
		Operation:         layers.ARPRequest,
		SourceHwAddress:   src,
		SourceProtAddress: srcIP.To4(),
		DstHwAddress:      []byte{0, 0, 0, 0, 0, 0},
		DstProtAddress:    nextHop.To4(),
	}
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, arp); err != nil {
		return nil, err
	}
	if err := handle.WritePacketData(buf.Bytes()); err != nil {
		return nil, fmt.Errorf("发送 ARP 请求失败: %w", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, eth, arp)
	decoded := make([]gopacket.LayerType, 0, 2)
	for time.Now().Before(deadline) {
		data, _, err := handle.ReadPacketData()
		if err != nil {
			time.Sleep(time.Millisecond)
			continue
		}
		decoded = decoded[:0]
		if perr := parser.DecodeLayers(data, &decoded); perr != nil {
			continue
		}
		for _, lt := range decoded {
			if lt == layers.LayerTypeARP &&
				arp.Operation == layers.ARPReply &&
				net.IP.Equal(arp.SourceProtAddress, nextHop.To4()) {
				return net.HardwareAddr(arp.SourceHwAddress), nil
			}
		}
	}
	return nil, fmt.Errorf("ARP timeout waiting for %s", nextHop)
}

// installRstDrop best-effort 安装「丢弃出站 RST」规则，规避内核对我们发出的
// SYN 的响应回 RST（保证 stealth）。安装失败仅告警，不影响正确性。
// 返回 cleanup 在扫描结束后卸载规则。
func installRstDrop(ctx context.Context, target, iface string) (func(), error) {
	// 防御性校验：target 必须是合法 IP（调用方已校验，此处纵深防御避免命令拼接风险）。
	if net.ParseIP(target) == nil {
		return nil, fmt.Errorf("installRstDrop: 非法目标 %q", target)
	}
	switch runtime.GOOS {
	case "linux":
		args := []string{"-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "RST", "RST", "-d", target, "-j", "DROP"}
		if err := exec.Command("iptables", args...).Run(); err != nil {
			return nil, fmt.Errorf("iptables 安装失败: %w", err)
		}
		return func() {
			_ = exec.Command("iptables", "-D", "OUTPUT", "-p", "tcp", "--tcp-flags", "RST", "RST", "-d", target, "-j", "DROP").Run()
		}, nil
	case "windows":
		args := []string{"advfirewall", "firewall", "add", "rule",
			"name=atlas-rst-drop", "dir=out", "action=block", "protocol=tcp", "remoteip=" + target}
		if err := exec.Command("netsh", args...).Run(); err != nil {
			return nil, fmt.Errorf("netsh 安装失败: %w", err)
		}
		return func() {
			_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name=atlas-rst-drop").Run()
		}, nil
	default:
		return nil, fmt.Errorf("平台 %s 不支持 RST-drop（仅告警）", runtime.GOOS)
	}
}
