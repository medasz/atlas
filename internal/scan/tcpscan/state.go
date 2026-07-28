package tcpscan

// State 端口状态（严格遵循 nmap 语义）。
// 不同扫描模式能得到的状态集合不同，详见 classify 与各模式说明。
type State string

const (
	Open         State = "open"          // SYN/connect 专属：端口在监听
	Closed       State = "closed"        // 可达但无服务（收到 RST）
	Filtered     State = "filtered"      // 被防火墙显式拒绝 / 静默丢弃（ICMP 不可达）
	Timeout      State = "timeout"       // 无响应（静默丢弃，区别于被显式过滤，利于诊断）
	OpenFiltered State = "open|filtered" // FIN/Null/Xmas 专属：开放或被过滤，无法区分
	Unfiltered   State = "unfiltered"    // ACK 专属：端口可达（防火墙放行 RST），不代表开放
)

// Result 单端口扫描结果。
type Result struct {
	Port   int
	State  State
	Banner string // 仅 open 且能读取到横幅时填充（connect 模式）
}

// classify 由原始回包推导端口状态，纯函数，不依赖任何特权，
// 因此可在无 root 环境下单测。
//
// 参数说明：
//   - flags：TCP 首部 flag 位（tcpFIN/tcpSYN/...），无回包时传 0。
//   - icmpUnreach：是否收到 ICMP 不可达（用于 filtered 判定，仅 IPv4 Type3）。
//   - mode：决定如何解释同一份回包语义。
//
// 设计依据（nmap TCP 扫描技术）：
//   - SYN / connect：SYN-ACK→open；RST→closed；ICMP 不可达→filtered；无响应→timeout。
//   - FIN / Null / Xmas：RST→closed；ICMP 不可达→filtered；无响应→open|filtered
//     （RFC 793：开放端口应忽略非常规探测，故与“被过滤”无法区分）。
//   - ACK：RST→unfiltered（防火墙放行）；ICMP 不可达或无响应→filtered（不探开放）。
func classify(flags uint8, icmpUnreach bool, mode Mode) State {
	switch mode {
	case ModeSyn, ModeConnect:
		switch {
		case icmpUnreach:
			return Filtered
		case flags&tcpSYN != 0 && flags&tcpACK != 0:
			return Open
		case flags&tcpRST != 0:
			return Closed
		default:
			return Timeout
		}
	case ModeFin, ModeNull, ModeXmas:
		switch {
		case flags&tcpRST != 0:
			return Closed
		case icmpUnreach:
			return Filtered
		default:
			return OpenFiltered
		}
	case ModeAck:
		// 仅 RST 表示可达；ICMP 不可达与无响应均判 filtered。
		if flags&tcpRST != 0 {
			return Unfiltered
		}
		return Filtered
	default:
		return Timeout
	}
}
