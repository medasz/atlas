package tcpscan

import "fmt"

// Mode 扫描模式。统一接口下由模式决定发包 flag 与响应语义。
type Mode string

const (
	ModeConnect Mode = "connect"
	ModeSyn     Mode = "syn"
	ModeAck     Mode = "ack"
	ModeFin     Mode = "fin"
	ModeNull    Mode = "null"
	ModeXmas    Mode = "xmas"
)

// TCP 首部 flag 位（数值与 gopacket layers.TCP 的常量一致）。
const (
	tcpFIN uint8 = 0x01
	tcpSYN uint8 = 0x02
	tcpRST uint8 = 0x04
	tcpPSH uint8 = 0x08
	tcpACK uint8 = 0x10
	tcpURG uint8 = 0x20
)

// flags 返回该模式发包时置位的 TCP flags。
// 仅 raw 模式有意义；connect 不走此路径（返回 0）。
func (m Mode) flags() uint8 {
	switch m {
	case ModeSyn:
		return tcpSYN
	case ModeFin:
		return tcpFIN
	case ModeNull:
		return 0
	case ModeXmas:
		return tcpFIN | tcpPSH | tcpURG
	case ModeAck:
		return tcpACK
	default:
		return 0
	}
}

// IsRaw 是否为 raw 包探测模式（需 gopacket + 抓包句柄）。
func (m Mode) IsRaw() bool {
	switch m {
	case ModeSyn, ModeAck, ModeFin, ModeNull, ModeXmas:
		return true
	default:
		return false
	}
}

// Valid 校验模式是否受支持。
func (m Mode) Valid() bool {
	switch m {
	case ModeConnect, ModeSyn, ModeAck, ModeFin, ModeNull, ModeXmas:
		return true
	}
	return false
}

// String 便于日志/调试。
func (m Mode) String() string { return string(m) }

// ParseMode 将字符串解析为 Mode，非法返回错误。
func ParseMode(s string) (Mode, error) {
	m := Mode(s)
	if !m.Valid() {
		return "", fmt.Errorf("unsupported scan mode: %q", s)
	}
	return m, nil
}
