package tcpscan

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestConnectScan(t *testing.T) {
	// 监听一个本地端口，应为 open
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("无法监听本地端口: %v", err)
	}
	defer ln.Close()
	openPort := ln.Addr().(*net.TCPAddr).Port

	// 一个极不可能在监听的端口，应为 closed
	closedPort := 1
	if openPort == 1 {
		closedPort = 2
	}

	sc := NewConnect()
	opts := Options{Timeout: 800 * time.Millisecond}
	res, err := sc.Scan(context.Background(), "127.0.0.1", []int{openPort, closedPort}, opts)
	if err != nil {
		t.Fatalf("Scan 返回错误: %v", err)
	}
	if res[openPort].State != Open {
		t.Errorf("端口 %d 期望 open, 实际 %q", openPort, res[openPort].State)
	}
	if res[closedPort].State != Closed {
		t.Errorf("端口 %d 期望 closed, 实际 %q", closedPort, res[closedPort].State)
	}
}

func TestConnectScanContextCancel(t *testing.T) {
	sc := NewConnect()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// 用一个极慢的超时避免真实连接；ctx 已取消应立即返回 ctx.Err
	res, err := sc.Scan(ctx, "127.0.0.1", []int{22}, Options{Timeout: 5 * time.Second})
	if err == nil {
		t.Error("已取消的 ctx 应返回错误")
	}
	if res == nil {
		t.Error("返回 map 不应为 nil")
	}
}
