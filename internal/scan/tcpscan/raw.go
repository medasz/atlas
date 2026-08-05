package tcpscan

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// linkLayer 抓包句柄的最小接口，便于用 mock 做单测。
// *pcap.Handle（capture 构建）天然满足该接口。
// 注意：pcap.Handle.Close() 无返回值，故此处 Close() 也不返回 error。
type linkLayer interface {
	WritePacketData([]byte) error
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
	LinkType() gopacket.Decoder
	Close()
}

// errRawUnavailable 表示 raw 抓包在当前环境不可用（无权限 / 无 pcap /
// 非 IPv4 等）。调用方据此降级为 connect（能力降级，非 OS 降级）。
type errRawUnavailable struct{ reason error }

func (e errRawUnavailable) Error() string { return "raw capture unavailable: " + e.reason.Error() }
func (e errRawUnavailable) Unwrap() error { return e.reason }

// IsRawUnavailable 判断错误是否为 raw 不可用（供调用方决定降级）。
func IsRawUnavailable(err error) bool {
	var e errRawUnavailable
	return errors.As(err, &e)
}

// rawScanner 基于 gopacket 的 raw 包探测实现（SYN/ACK/FIN/Null/Xmas）。
// 模式差异仅体现在发包 TCP flags，响应分类逻辑共享。
type rawScanner struct{ mode Mode }

// NewRaw 返回指定 raw 模式的 Scanner。
func NewRaw(mode Mode) Scanner { return rawScanner{mode: mode} }

func (r rawScanner) Mode() Mode { return r.mode }

// Scan 对单一目标 IP 执行 raw 包扫描：
//  1. 解析出口网卡 / 源 IP；
//  2. 打开抓包句柄（能力检查点，失败 → 降级 connect）；
//  3. best-effort 安装 RST-drop 规则规避内核回 RST；
//  4. 解析目标 MAC（ARP）；
//  5. 整块端口广发探测包 + 一个抓包窗口回收响应。
func (r rawScanner) Scan(ctx context.Context, target string, ports []int, opts Options) (map[int]Result, error) {
	dstIP := net.ParseIP(target)
	if dstIP == nil || dstIP.To4() == nil {
		err := fmt.Errorf("raw scan 仅支持 IPv4 目标，收到 %q", target)
		if opts.OnRawFallback != nil {
			opts.OnRawFallback(err)
		}
		return nil, errRawUnavailable{err}
	}

	route, err := resolveRawRoute(dstIP, opts.Iface)
	if err != nil {
		if opts.OnRawFallback != nil {
			opts.OnRawFallback(err)
		}
		return nil, errRawUnavailable{err}
	}
	srcIP := route.SourceIP
	if opts.SourceIP != nil {
		srcIP = opts.SourceIP.To4()
		if srcIP == nil {
			err := fmt.Errorf("raw scan source IP must be IPv4: %s", opts.SourceIP)
			if opts.OnRawFallback != nil {
				opts.OnRawFallback(err)
			}
			return nil, errRawUnavailable{err}
		}
	}

	handle, err := openHandle(route.Iface, opts)
	if err != nil {
		if opts.OnRawFallback != nil {
			opts.OnRawFallback(err)
		}
		return nil, errRawUnavailable{err}
	}
	defer handle.Close()

	srcMAC := ifaceMAC(route.Iface)
	if len(srcMAC) == 0 {
		err := fmt.Errorf("interface %s has no hardware address", route.Iface)
		if opts.OnRawFallback != nil {
			opts.OnRawFallback(err)
		}
		return nil, errRawUnavailable{err}
	}
	dstMAC, err := resolveMAC(handle, route.Iface, srcIP, route.NextHop)
	if err != nil {
		err = fmt.Errorf("resolve next-hop MAC %s for target %s: %w", route.NextHop, target, err)
		if opts.OnRawFallback != nil {
			opts.OnRawFallback(err)
		}
		return nil, errRawUnavailable{err}
	}

	if opts.InstallRstDrop {
		if cleanup, rerr := installRstDrop(ctx, target, route.Iface); rerr != nil {
			log.Printf("tcpscan: 安装 RST-drop 规则失败（仅影响 stealth，不影响正确性）: %v", rerr)
		} else if cleanup != nil {
			defer cleanup()
		}
	}

	window := opts.Timeout
	if window <= 0 {
		window = 3 * time.Second
	}
	srcPort := pickSrcPort(opts)
	sendProbes(handle, srcIP, srcMAC, dstMAC, srcPort, dstIP, ports, r.mode.flags(), opts.Retries)
	return captureResponses(ctx, handle, dstIP, srcPort, ports, r.mode, window), nil
}

// sendProbes 将整块端口的探测包发出（retries+1 轮广发）。
func sendProbes(handle linkLayer, srcIP net.IP, srcMAC, dstMAC net.HardwareAddr, srcPort uint16, dstIP net.IP, ports []int, flags uint8, retries int) {
	sends := retries + 1
	for i := 0; i < sends; i++ {
		for _, p := range ports {
			b, err := buildProbe(srcIP, srcMAC, dstMAC, srcPort, dstIP, uint16(p), flags)
			if err != nil {
				continue
			}
			_ = handle.WritePacketData(b)
		}
	}
}

// captureResponses 在窗口内抓包并按响应推导各端口状态。
// 匹配依据：响应 TCP.DstPort == 我们的 srcPort，且响应 TCP.SrcPort == 被扫端口。
func captureResponses(ctx context.Context, handle linkLayer, targetIP net.IP, srcPort uint16, ports []int, mode Mode, window time.Duration) map[int]Result {
	results := make(map[int]Result, len(ports))
	portSet := make(map[int]struct{}, len(ports))
	for _, p := range ports {
		results[p] = Result{Port: p, State: Timeout}
		portSet[p] = struct{}{}
	}

	deadline := time.Now().Add(window)
	debug := os.Getenv("ATLAS_RAW_DEBUG") == "1"
	loggedDecodeError := false

	for time.Now().Before(deadline) && ctx.Err() == nil {
		data, _, err := handle.ReadPacketData()
		if err != nil {
			time.Sleep(time.Millisecond)
			continue
		}
		packet := gopacket.NewPacket(data, handle.LinkType(), gopacket.NoCopy)
		if perr := packet.ErrorLayer(); perr != nil {
			if debug && !loggedDecodeError {
				log.Printf("tcpscan: raw debug decode failed link_type=%T bytes=%d: %v", handle.LinkType(), len(data), perr.Error())
				loggedDecodeError = true
			}
			continue
		}

		if layer := packet.Layer(layers.LayerTypeTCP); layer != nil {
			tcp := layer.(*layers.TCP)
			if tcp.DstPort == layers.TCPPort(srcPort) {
				if debug {
					var source, destination net.IP
					if ipLayer := packet.Layer(layers.LayerTypeIPv4); ipLayer != nil {
						ip := ipLayer.(*layers.IPv4)
						source, destination = ip.SrcIP, ip.DstIP
					}
					log.Printf("tcpscan: raw debug TCP src=%s:%d dst=%s:%d flags=%#x", source, tcp.SrcPort, destination, tcp.DstPort, tcpFlagBits(tcp))
				}
				if dport := int(tcp.SrcPort); dport != 0 {
					if _, ok := portSet[dport]; ok {
						results[dport] = Result{Port: dport, State: classify(tcpFlagBits(tcp), false, mode)}
					}
				}
			}
		}

		if layer := packet.Layer(layers.LayerTypeICMPv4); layer != nil {
			icmp := layer.(*layers.ICMPv4)
			if icmpUnreachable(icmp) {
				if dport, ok := icmpOriginalDstPort(icmp); ok {
					if _, ok := portSet[dport]; ok {
						results[dport] = Result{Port: dport, State: classify(0, true, mode)}
					}
				}
			}
		}
	}

	// 窗口结束仍未收到任何响应的端口，按模式补全语义：
	// FIN/Null/Xmas → open|filtered；ACK → filtered；SYN → 维持 timeout。
	for p := range results {
		if results[p].State != Timeout {
			continue
		}
		switch mode {
		case ModeFin, ModeNull, ModeXmas:
			results[p] = Result{Port: p, State: OpenFiltered}
		case ModeAck:
			results[p] = Result{Port: p, State: Filtered}
		}
	}
	return results
}

// buildProbe 构造一个 IPv4+TCP 探测帧（已含 Ethernet 头，便于 pcap 直发）。
func buildProbe(srcIP net.IP, srcMAC, dstMAC net.HardwareAddr, srcPort uint16, dstIP net.IP, dstPort uint16, flags uint8) ([]byte, error) {
	eth := &layers.Ethernet{
		SrcMAC:       srcMAC,
		DstMAC:       dstMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    srcIP,
		DstIP:    dstIP,
	}
	tcp := &layers.TCP{
		SrcPort: layers.TCPPort(srcPort),
		DstPort: layers.TCPPort(dstPort),
		Seq:     rand.Uint32(),
		Window:  64240,
	}
	setFlags(tcp, flags)
	tcp.SetNetworkLayerForChecksum(ip)

	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func setFlags(t *layers.TCP, flags uint8) {
	t.FIN = flags&tcpFIN != 0
	t.SYN = flags&tcpSYN != 0
	t.RST = flags&tcpRST != 0
	t.PSH = flags&tcpPSH != 0
	t.ACK = flags&tcpACK != 0
	t.URG = flags&tcpURG != 0
}

func tcpFlagBits(t *layers.TCP) uint8 {
	var f uint8
	if t.FIN {
		f |= tcpFIN
	}
	if t.SYN {
		f |= tcpSYN
	}
	if t.RST {
		f |= tcpRST
	}
	if t.PSH {
		f |= tcpPSH
	}
	if t.ACK {
		f |= tcpACK
	}
	if t.URG {
		f |= tcpURG
	}
	return f
}

func icmpUnreachable(ic *layers.ICMPv4) bool {
	return ic.TypeCode.Type() == layers.ICMPv4TypeDestinationUnreachable
}

// icmpOriginalDstPort 从 ICMP 不可达载荷里解析出原始探测的目标端口。
func icmpOriginalDstPort(ic *layers.ICMPv4) (int, bool) {
	payload := ic.LayerPayload()
	if len(payload) == 0 {
		return 0, false
	}
	pkt := gopacket.NewPacket(payload, layers.LayerTypeIPv4, gopacket.DecodeOptions{})
	if t := pkt.Layer(layers.LayerTypeTCP); t != nil {
		return int(t.(*layers.TCP).DstPort), true
	}
	return 0, false
}

func pickSrcPort(opts Options) uint16 {
	if opts.SrcPort > 0 && opts.SrcPort <= 65535 {
		return uint16(opts.SrcPort)
	}
	return uint16(32768 + rand.Intn(1<<15))
}

func ifaceMAC(name string) net.HardwareAddr {
	if iface, err := net.InterfaceByName(name); err == nil {
		return iface.HardwareAddr
	}
	return nil
}
