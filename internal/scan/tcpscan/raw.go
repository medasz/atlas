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

type rawSendStats struct {
	Attempted   int
	Written     int
	WriteErrors int
}

type rawCaptureStats struct {
	FramesRead        int
	DecodeErrors      int
	TCPToSourcePort   int
	TCPForeignSource  int
	TCPMatchedPort    int
	TCPUnmatchedPort  int
	SYNACK            int
	RST               int
	ICMPUnreachable   int
	ICMPMatchedPort   int
	ICMPUnmatchedPort int
}

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
	log.Printf("tcpscan: raw path task_id=%s port_range=%s target=%s mode=%s iface=%s source_ip=%s next_hop=%s source_port=%d target_mac=%s ports_count=%d retries=%d capture_window_ms=%d",
		traceValue(opts.TaskID), traceValue(opts.PortRange), target, r.mode, route.Iface, srcIP, route.NextHop, srcPort, dstMAC, len(ports), opts.Retries, window.Milliseconds())
	startedAt := time.Now()
	sent := sendProbes(handle, srcIP, srcMAC, dstMAC, srcPort, dstIP, ports, r.mode.flags(), opts.Retries)
	results, captured := captureResponsesWithStats(ctx, handle, dstIP, srcPort, ports, r.mode, window)
	states := countStates(results)
	log.Printf("tcpscan: raw result task_id=%s port_range=%s target=%s mode=%s duration_ms=%d probe_attempted=%d frames_written=%d write_errors=%d frames_read=%d tcp_to_source_port=%d tcp_foreign_source=%d tcp_matched_port=%d tcp_unmatched_port=%d syn_ack=%d rst=%d icmp_unreachable=%d icmp_matched_port=%d icmp_unmatched_port=%d decode_errors=%d state_open=%d state_closed=%d state_filtered=%d state_timeout=%d state_open_filtered=%d state_unfiltered=%d",
		traceValue(opts.TaskID), traceValue(opts.PortRange), target, r.mode, time.Since(startedAt).Milliseconds(), sent.Attempted, sent.Written, sent.WriteErrors, captured.FramesRead, captured.TCPToSourcePort, captured.TCPForeignSource, captured.TCPMatchedPort, captured.TCPUnmatchedPort, captured.SYNACK, captured.RST, captured.ICMPUnreachable, captured.ICMPMatchedPort, captured.ICMPUnmatchedPort, captured.DecodeErrors, states[Open], states[Closed], states[Filtered], states[Timeout], states[OpenFiltered], states[Unfiltered])
	if captured.TCPMatchedPort+captured.ICMPMatchedPort == 0 {
		log.Printf("tcpscan: raw warning task_id=%s port_range=%s target=%s mode=%s no matched replies captured; open|filtered or filtered results may reflect a silent target, filtering, or an asymmetric return path", traceValue(opts.TaskID), traceValue(opts.PortRange), target, r.mode)
	}
	return results, nil
}

// sendProbes 将整块端口的探测包发出（retries+1 轮广发）。
func sendProbes(handle linkLayer, srcIP net.IP, srcMAC, dstMAC net.HardwareAddr, srcPort uint16, dstIP net.IP, ports []int, flags uint8, retries int) rawSendStats {
	stats := rawSendStats{}
	sends := retries + 1
	for i := 0; i < sends; i++ {
		for _, p := range ports {
			stats.Attempted++
			b, err := buildProbe(srcIP, srcMAC, dstMAC, srcPort, dstIP, uint16(p), flags)
			if err != nil {
				stats.WriteErrors++
				continue
			}
			if err := handle.WritePacketData(b); err != nil {
				stats.WriteErrors++
				continue
			}
			stats.Written++
		}
	}
	return stats
}

// captureResponses 在窗口内抓包并按响应推导各端口状态。
// 匹配依据：响应 TCP.DstPort == 我们的 srcPort，且响应 TCP.SrcPort == 被扫端口。
func captureResponses(ctx context.Context, handle linkLayer, targetIP net.IP, srcPort uint16, ports []int, mode Mode, window time.Duration) map[int]Result {
	results, _ := captureResponsesWithStats(ctx, handle, targetIP, srcPort, ports, mode, window)
	return results
}

func captureResponsesWithStats(ctx context.Context, handle linkLayer, targetIP net.IP, srcPort uint16, ports []int, mode Mode, window time.Duration) (map[int]Result, rawCaptureStats) {
	results := make(map[int]Result, len(ports))
	stats := rawCaptureStats{}
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
		stats.FramesRead++
		packet := gopacket.NewPacket(data, handle.LinkType(), gopacket.NoCopy)
		if perr := packet.ErrorLayer(); perr != nil {
			stats.DecodeErrors++
			if debug && !loggedDecodeError {
				log.Printf("tcpscan: raw debug decode failed link_type=%T bytes=%d: %v", handle.LinkType(), len(data), perr.Error())
				loggedDecodeError = true
			}
			continue
		}

		if layer := packet.Layer(layers.LayerTypeTCP); layer != nil {
			tcp := layer.(*layers.TCP)
			if tcp.DstPort == layers.TCPPort(srcPort) {
				stats.TCPToSourcePort++
				ipLayer := packet.Layer(layers.LayerTypeIPv4)
				if ipLayer == nil || !ipLayer.(*layers.IPv4).SrcIP.Equal(targetIP) {
					stats.TCPForeignSource++
					continue
				}
				if debug {
					ip := ipLayer.(*layers.IPv4)
					source, destination := ip.SrcIP, ip.DstIP
					log.Printf("tcpscan: raw debug TCP src=%s:%d dst=%s:%d flags=%#x", source, tcp.SrcPort, destination, tcp.DstPort, tcpFlagBits(tcp))
				}
				if dport := int(tcp.SrcPort); dport != 0 {
					if _, ok := portSet[dport]; ok {
						flags := tcpFlagBits(tcp)
						stats.TCPMatchedPort++
						if flags&tcpSYN != 0 && flags&tcpACK != 0 {
							stats.SYNACK++
						}
						if flags&tcpRST != 0 {
							stats.RST++
						}
						results[dport] = Result{Port: dport, State: classify(flags, false, mode)}
					} else {
						stats.TCPUnmatchedPort++
					}
				}
			}
		}

		if layer := packet.Layer(layers.LayerTypeICMPv4); layer != nil {
			icmp := layer.(*layers.ICMPv4)
			if icmpUnreachable(icmp) {
				stats.ICMPUnreachable++
				if dport, ok := icmpOriginalDstPort(icmp); ok {
					if _, ok := portSet[dport]; ok {
						stats.ICMPMatchedPort++
						results[dport] = Result{Port: dport, State: classify(0, true, mode)}
					} else {
						stats.ICMPUnmatchedPort++
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
	return results, stats
}

func countStates(results map[int]Result) map[State]int {
	counts := make(map[State]int)
	for _, result := range results {
		counts[result.State]++
	}
	return counts
}

func traceValue(value string) string {
	if value == "" {
		return "-"
	}
	return value
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
