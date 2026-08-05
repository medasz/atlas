//go:build integration

package tcpscan

import (
	"context"
	"net"
	"os"
	"runtime"
	"strconv"
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

// TestIntegrationRawSynTarget is opt-in so CI never probes external targets.
// It is used to verify the real raw packet path from a deployment environment.
func TestIntegrationRawSynTarget(t *testing.T) {
	target := os.Getenv("ATLAS_RAW_TEST_TARGET")
	if target == "" {
		t.Skip("set ATLAS_RAW_TEST_TARGET to run a real raw SYN probe")
	}
	port := 22
	if value := os.Getenv("ATLAS_RAW_TEST_PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 65535 {
			t.Fatalf("invalid ATLAS_RAW_TEST_PORT %q", value)
		}
		port = parsed
	}

	scanner, err := New(ModeSyn, Options{Timeout: 3 * time.Second, Retries: 1, InstallRstDrop: true})
	if err != nil {
		t.Fatal(err)
	}
	results, err := scanner.Scan(context.Background(), target, []int{port}, Options{
		Timeout: 3 * time.Second, Retries: 1, InstallRstDrop: true,
	})
	if err != nil {
		t.Fatalf("raw SYN scan failed: %v", err)
	}
	if got := results[port].State; got != Open {
		t.Fatalf("raw SYN scan %s:%d = %s, want open", target, port, got)
	}
}
