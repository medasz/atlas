package tcpscan

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// ---- mock linkLayer：按预设顺序回放数据包，耗尽后返回超时错误 ----

type mockTimeout struct{}

func (mockTimeout) Error() string { return "timeout" }
func (mockTimeout) Timeout() bool { return true }

type mockHandle struct {
	packets [][]byte
	idx     int
	link    gopacket.Decoder
}

func (m *mockHandle) WritePacketData(b []byte) error { return nil }
func (m *mockHandle) LinkType() gopacket.Decoder     { return m.link }
func (m *mockHandle) Close()                         {}
func (m *mockHandle) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	if m.idx >= len(m.packets) {
		return nil, gopacket.CaptureInfo{}, mockTimeout{}
	}
	p := m.packets[m.idx]
	m.idx++
	return p, gopacket.CaptureInfo{}, nil
}

// makeResp 构造一个「响应」帧：SrcPort=被扫端口，DstPort=我们的 srcPort。
func makeResp(srcMAC, dstMAC net.HardwareAddr, srcIP, dstIP net.IP, srcPort, dstPort uint16, flags uint8) []byte {
	eth := &layers.Ethernet{SrcMAC: srcMAC, DstMAC: dstMAC, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: srcIP, DstIP: dstIP}
	tcp := &layers.TCP{SrcPort: layers.TCPPort(srcPort), DstPort: layers.TCPPort(dstPort), Seq: 1, Window: 1024}
	setFlags(tcp, flags)
	tcp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	_ = gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp)
	return buf.Bytes()
}

// makeRespNoEth 构造无 Ethernet 头的 IPv4+TCP 帧（用于 ICMP 不可达载荷）。
func makeRespNoEth(srcIP, dstIP net.IP, srcPort, dstPort uint16, flags uint8) []byte {
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: srcIP, DstIP: dstIP}
	tcp := &layers.TCP{SrcPort: layers.TCPPort(srcPort), DstPort: layers.TCPPort(dstPort), Seq: 1, Window: 1024}
	setFlags(tcp, flags)
	tcp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	_ = gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ip, tcp)
	return buf.Bytes()
}

// makeICMPUnreach 构造 ICMP 不可达帧，载荷为原始探测包（纯 IPv4+TCP，不含 Ethernet）。
func makeICMPUnreach(srcMAC, dstMAC net.HardwareAddr, srcIP, dstIP net.IP, payload []byte) []byte {
	eth := &layers.Ethernet{SrcMAC: srcMAC, DstMAC: dstMAC, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolICMPv4, SrcIP: srcIP, DstIP: dstIP}
	icmp := &layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeDestinationUnreachable, 1)}
	buf := gopacket.NewSerializeBuffer()
	_ = gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, icmp, gopacket.Payload(payload))
	return buf.Bytes()
}

var testMAC = net.HardwareAddr{0xaa, 0, 0, 0, 0, 0}

func TestCaptureResponsesSyn(t *testing.T) {
	target := net.ParseIP("10.0.0.5")
	srcPort := uint16(40000)
	pkts := [][]byte{
		makeResp(testMAC, testMAC, target, net.ParseIP("10.0.0.1"), uint16(80), srcPort, tcpSYN|tcpACK),
		makeResp(testMAC, testMAC, target, net.ParseIP("10.0.0.1"), uint16(81), srcPort, tcpRST),
	}
	h := &mockHandle{packets: pkts, link: layers.LinkTypeEthernet}
	res := captureResponses(context.Background(), h, target, srcPort, []int{80, 81, 82}, ModeSyn, 150*time.Millisecond)
	if res[80].State != Open {
		t.Errorf("端口80 期望 open, 实际 %s", res[80].State)
	}
	if res[81].State != Closed {
		t.Errorf("端口81 期望 closed, 实际 %s", res[81].State)
	}
	if res[82].State != Timeout {
		t.Errorf("端口82 期望 timeout（无响应）, 实际 %s", res[82].State)
	}
}

func TestCaptureResponsesFinNoReply(t *testing.T) {
	target := net.ParseIP("10.0.0.5")
	srcPort := uint16(40000)
	// 完全无响应：FIN 模式应判 open|filtered
	h := &mockHandle{packets: nil, link: layers.LinkTypeEthernet}
	res := captureResponses(context.Background(), h, target, srcPort, []int{80}, ModeFin, 120*time.Millisecond)
	if res[80].State != OpenFiltered {
		t.Errorf("FIN 无响应期望 open|filtered, 实际 %s", res[80].State)
	}
}

func TestCaptureResponsesAckNoReply(t *testing.T) {
	target := net.ParseIP("10.0.0.5")
	srcPort := uint16(40000)
	h := &mockHandle{packets: nil, link: layers.LinkTypeEthernet}
	res := captureResponses(context.Background(), h, target, srcPort, []int{80}, ModeAck, 120*time.Millisecond)
	if res[80].State != Filtered {
		t.Errorf("ACK 无响应期望 filtered, 实际 %s", res[80].State)
	}
}

func TestCaptureResponsesICMPFiltered(t *testing.T) {
	target := net.ParseIP("10.0.0.5")
	gw := net.ParseIP("10.0.0.1")
	srcPort := uint16(40000)
	// 原始探测包（ICMP 载荷，纯 IPv4+TCP）：源端口=srcPort，目的端口=80
	orig := makeRespNoEth(gw, target, srcPort, uint16(80), tcpSYN)
	// ICMP 不可达由 target 发回（SrcIP=target, DstIP=gw）
	pkt := makeICMPUnreach(testMAC, testMAC, target, gw, orig)
	h := &mockHandle{packets: [][]byte{pkt}, link: layers.LinkTypeEthernet}
	res := captureResponses(context.Background(), h, target, srcPort, []int{80, 81}, ModeSyn, 150*time.Millisecond)
	if res[80].State != Filtered {
		t.Errorf("端口80 收到 ICMP 不可达期望 filtered, 实际 %s", res[80].State)
	}
}

func TestBuildProbeFlags(t *testing.T) {
	srcIP, dstIP := net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.5")
	b, err := buildProbe(srcIP, testMAC, testMAC, 40000, dstIP, 80, ModeSyn.flags())
	if err != nil {
		t.Fatal(err)
	}
	pkt := gopacket.NewPacket(b, layers.LayerTypeEthernet, gopacket.DecodeOptions{})
	tcp := pkt.Layer(layers.LayerTypeTCP).(*layers.TCP)
	if !tcp.SYN || tcp.ACK || tcp.RST || tcp.FIN {
		t.Errorf("SYN 探测应仅置 SYN，实际 FIN=%v SYN=%v RST=%v ACK=%v", tcp.FIN, tcp.SYN, tcp.RST, tcp.ACK)
	}

	b2, _ := buildProbe(srcIP, testMAC, testMAC, 40000, dstIP, 80, ModeXmas.flags())
	tcp2 := gopacket.NewPacket(b2, layers.LayerTypeEthernet, gopacket.DecodeOptions{}).Layer(layers.LayerTypeTCP).(*layers.TCP)
	if !tcp2.FIN || !tcp2.PSH || !tcp2.URG || tcp2.SYN {
		t.Errorf("Xmas 探测 flags 错误: FIN=%v PSH=%v URG=%v SYN=%v", tcp2.FIN, tcp2.PSH, tcp2.URG, tcp2.SYN)
	}

	b3, _ := buildProbe(srcIP, testMAC, testMAC, 40000, dstIP, 80, ModeNull.flags())
	tcp3 := gopacket.NewPacket(b3, layers.LayerTypeEthernet, gopacket.DecodeOptions{}).Layer(layers.LayerTypeTCP).(*layers.TCP)
	if tcp3.FIN || tcp3.SYN || tcp3.RST || tcp3.PSH || tcp3.ACK || tcp3.URG {
		t.Errorf("Null 探测应无任何 flag，实际 FIN=%v SYN=%v RST=%v PSH=%v ACK=%v URG=%v",
			tcp3.FIN, tcp3.SYN, tcp3.RST, tcp3.PSH, tcp3.ACK, tcp3.URG)
	}
}
