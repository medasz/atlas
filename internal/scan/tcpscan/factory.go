package tcpscan

import "fmt"

// New 根据模式创建对应的 Scanner。
//   - connect → connectScanner（复用 OS TCP 栈，无需特权）
//   - syn/ack/fin/null/xmas → rawScanner（gopacket raw 包）
//
// raw 模式在 Scan 时若抓包句柄不可用（无权限 / 缺 Npcap 等）会返回
// errRawUnavailable，由调用方据此降级为 connect（能力降级，非 OS 降级）。
// 非法模式返回 error。
func New(mode Mode, opts Options) (Scanner, error) {
	if !mode.Valid() {
		return nil, fmt.Errorf("unsupported scan mode: %q", mode)
	}
	switch {
	case mode == ModeConnect:
		return NewConnect(), nil
	default:
		return NewRaw(mode), nil
	}
}
