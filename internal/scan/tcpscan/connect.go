package tcpscan

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"
)

// connectScanner 全连接扫描：复用 OS TCP 栈（net.DialTimeout），
// 无 raw 依赖、无需特权。仅能判定 open / closed（无法区分 filtered/timeout）。
type connectScanner struct{}

// NewConnect 返回 connect 模式的 Scanner。
func NewConnect() Scanner { return connectScanner{} }

func (connectScanner) Mode() Mode { return ModeConnect }

func (c connectScanner) Scan(ctx context.Context, target string, ports []int, opts Options) (map[int]Result, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	out := make(map[int]Result, len(ports))
	for _, p := range ports {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}
		conn, err := c.dial(ctx, target, p, timeout)
		if err != nil {
			// connect 模式无法区分 RST / 超时 / 不可达，统一记为 closed（与原 tcpConnect 行为一致）。
			out[p] = Result{Port: p, State: Closed}
			continue
		}
		banner := grabBanner(conn, timeout)
		_ = conn.Close()
		out[p] = Result{Port: p, State: Open, Banner: banner}
	}
	return out, nil
}

func (connectScanner) dial(ctx context.Context, ip string, port int, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	return d.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
}

func grabBanner(conn net.Conn, timeout time.Duration) string {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return ""
	}
	return strings.TrimSpace(string(buf[:n]))
}
