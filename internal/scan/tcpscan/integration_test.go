//go:build integration

package tcpscan

import (
	"context"
	"net"
	"runtime"
	"testing"
	"time"
)

// 本文件为集成测试，需显式构建：go test -tags integration ./internal/scan/tcpscan/
// 其中 raw 模式还需 -tags raw_capture 且具备特权（Linux root / Windows 管理员+Npcap）。

func TestIntegrationConnectLocalhost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("无法监听本地端口: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	sc, err := New(ModeConnect, Options{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	res, err := sc.Scan(context.Background(), "127.0.0.1", []int{port, 1}, Options{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("connect 扫描失败: %v", err)
	}
	if res[port].State != Open {
		t.Errorf("端口 %d 期望 open, 实际 %s", port, res[port].State)
	}
	if res[1].State != Closed {
		t.Errorf("端口 1 期望 closed, 实际 %s", res[1].State)
	}
}

func TestIntegrationRawSynLocalhost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 需 Npcap + 管理员权限，CI 跳过")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("无法监听本地端口: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	sc, err := New(ModeSyn, Options{Timeout: 3 * time.Second, Retries: 1, InstallRstDrop: false})
	if err != nil {
		t.Fatal(err)
	}
	res, err := sc.Scan(context.Background(), "127.0.0.1", []int{port}, Options{Timeout: 3 * time.Second, Retries: 1, InstallRstDrop: false})
	if err != nil {
		// 无特权 / 缺抓包句柄 → 优雅降级或跳过
		t.Skipf("raw SYN 不可用（需 root / Npcap）: %v", err)
	}
	if res[port].State != Open {
		t.Errorf("raw SYN 扫描端口 %d 期望 open, 实际 %s", port, res[port].State)
	}
}
