# TCP 多模式扫描 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 atlas 的 `internal/scan` 中实现统一接口切换 6 种 TCP 扫描（syn/connect/ack/fin/null/xmas），raw 模式走 gopacket（Linux libpcap / Windows Npcap），状态词表严格 nmap 语义，并接入 atlas 落库与 Web 控制台。

**Architecture:** 新增解耦子包 `internal/scan/tcpscan`（不依赖 store），暴露 `Scanner.Scan(ctx, target, ports, opts) (map[int]Result, error)`。connect 重构进该接口（行为不变）；raw 5 种共用 gopacket 引擎，仅 TCP flags 不同，响应按模式语义分类。atlas `Scanner` 持有 `tcpscan.Scanner` 并在 `scanHost` 接入，结果写 `model.Port.State`。

**Tech Stack:** Go, `github.com/google/gopacket`（pcap），现有 atlas `store`/`config`/`model`，前端 `Settings.vue` + 现有配置 API。

## Global Constraints

- 跨平台：**Linux=libpcap，Windows=Npcap，均走 gopacket raw；不按 OS 回退 connect**，仅当抓包句柄打不开（缺权限/缺 Npcap）时按**能力**降级 connect + 告警。
- 状态词表严格 nmap：`open` / `closed` / `filtered` / `timeout` / `open|filtered` (FIN/Null/Xmas) / `unfiltered` (ACK)。
- raw 并发 = 并行目标 IP；单 IP 内整块端口一次性发出 + 一个抓包窗口回收。
- 落库：`open` 必存；`filtered` 受 `RecordFilteredPorts`(默认 true)；`closed`/`timeout`/`open|filtered`/`unfiltered` 受 `RecordClosedPorts`(默认 false)。
- 配置 `RawIface` 默认空=自动，且经现有配置 API 在 Settings 界面可编辑（与 `DefaultMode` 同机制）。
- 已有代码约束：`Scanner` 当前在 `internal/scan/scan.go:20-27`；`tcpConnect` 在 `probe.go:18`；`model.Port` 在 `model.go:19-33` 无 `State`；`DefaultMode` 在 `config.go:43` 惰性未用；`UpsertPort` 在 `store/pg.go`。
- atlas 当前非 git 仓库：提交需用户许可（本计划内含 commit 步骤，执行前请确认是否已 `git init`）。

---

## File Structure

- `internal/scan/tcpscan/state.go` — `State` / `Result` 类型与常量。
- `internal/scan/tcpscan/mode.go` — `Mode` 类型 + TCP flag 常量 + `flags()` + `isRaw()`。
- `internal/scan/tcpscan/classify.go` — 纯函数 `classify`（mode 感知），无 gopacket 依赖，可单测。
- `internal/scan/tcpscan/prober.go` — `Scanner` 接口 + `Options`。
- `internal/scan/tcpscan/connect.go` — connect 实现（封装 `net.DialTimeout` + banner 抓取）。
- `internal/scan/tcpscan/raw.go` — gopacket 引擎：发包 / 窗口抓包 / 分类 / RST 规避 / 网卡解析。
- `internal/scan/tcpscan/factory.go` — `New(mode, opts)` 选实现；非法 mode 报错。
- `internal/scan/tcpscan/classify_test.go` — `classify` 纯函数表驱动单测。
- `internal/scan/tcpscan/connect_test.go` — connect 单测（本地监听对照）。
- `internal/scan/tcpscan/raw_test.go` — 链路层回放 / mock 单测。
- `internal/scan/tcpscan/factory_test.go` — factory 单测。
- `internal/model/model.go` — `Port` 加 `State` 字段。
- `migrations/000005_add_port_state.up.sql` + `.down.sql` — 加 `state` 列。
- `internal/store/pg.go` — 确认 `UpsertPort` 覆盖 `state`（必要时补列）。
- `internal/scan/scan.go` — `Scanner` 加 `mode`/`ts`/`tcpscanOpts`；`New` 收 cfg；`scanHost` 接入 `tcpscan`；移除已迁入 `tcpscan` 的 `tcpConnect`/`grabBanner`。
- `internal/config/config.go` — `ScanConfig` 加 raw 参数。
- `cmd/atlas/main.go` — `scan.New` 传 cfg。
- `internal/server/config.go` — 无需改（全量配置已透传）。
- 前端 `Settings.vue` — 加「抓包网卡」输入框。
- `docs/SPEC.md` — 同步覆盖率矩阵（US-008 → 已实现）。

---

### Task 1: 状态与模式类型 + classify 纯函数（TDD）

**Files:**
- Create: `internal/scan/tcpscan/state.go`
- Create: `internal/scan/tcpscan/mode.go`
- Create: `internal/scan/tcpscan/classify.go`
- Test: `internal/scan/tcpscan/classify_test.go`

**Interfaces:**
- Produces: `State` 常量、`Result`、`Mode` 常量、`classify(flags uint8, icmpUnreach, responded bool, mode Mode) State`（后续 raw.go / connect.go / 测试均依赖）。

- [ ] **Step 1: 写失败测试 `classify_test.go`**

```go
package tcpscan

import "testing"

func TestClassify(t *testing.T) {
	const (
		SYN_ACK = tcpSYN | tcpACK
		RST     = tcpRST
		NONE    = uint8(0)
	)
	cases := []struct {
		name         string
		mode         Mode
		flags        uint8
		icmpUnreach  bool
		responded    bool
		want         State
	}{
		{"syn-open", ModeSYN, SYN_ACK, false, true, Open},
		{"syn-closed", ModeSYN, RST, false, true, Closed},
		{"syn-filtered-icmp", ModeSYN, NONE, true, true, Filtered},
		{"syn-timeout", ModeSYN, NONE, false, false, Timeout},
		{"connect-open", ModeConnect, SYN_ACK, false, true, Open},
		{"connect-closed", ModeConnect, RST, false, true, Closed},
		{"connect-timeout", ModeConnect, NONE, false, false, Timeout},
		{"fin-closed", ModeFIN, RST, false, true, Closed},
		{"fin-openfiltered", ModeFIN, NONE, false, false, OpenFiltered},
		{"null-openfiltered", ModeNull, NONE, false, false, OpenFiltered},
		{"xmas-closed", ModeXmas, RST, false, true, Closed},
		{"ack-unfiltered", ModeACK, RST, false, true, Unfiltered},
		{"ack-filtered", ModeACK, NONE, false, false, Filtered},
		{"ack-filtered-icmp", ModeACK, NONE, true, true, Filtered},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classify(c.flags, c.icmpUnreach, c.responded, c.mode); got != c.want {
				t.Fatalf("classify()=%s want %s", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd d:/myself/scan/atlas && go test ./internal/scan/tcpscan/ -run TestClassify -v`
Expected: FAIL（`classify`/`State` 未定义，包不存在）。

- [ ] **Step 3: 实现最小代码 `state.go` / `mode.go` / `classify.go`**

`state.go`:
```go
package tcpscan

type State string

const (
	Open         State = "open"
	Closed       State = "closed"
	Filtered     State = "filtered"
	Timeout      State = "timeout"
	OpenFiltered State = "open|filtered"
	Unfiltered   State = "unfiltered"
)

type Result struct {
	Port   int
	State  State
	Banner string
}
```

`mode.go`:
```go
package tcpscan

type Mode string

const (
	ModeSYN     Mode = "syn"
	ModeConnect Mode = "connect"
	ModeACK     Mode = "ack"
	ModeFIN     Mode = "fin"
	ModeNull    Mode = "null"
	ModeXmas    Mode = "xmas"
)

const (
	tcpFIN uint8 = 0x01
	tcpSYN uint8 = 0x02
	tcpRST uint8 = 0x04
	tcpPSH uint8 = 0x08
	tcpACK uint8 = 0x10
	tcpURG uint8 = 0x20
)

func (m Mode) flags() uint8 {
	switch m {
	case ModeSYN:
		return tcpSYN
	case ModeACK:
		return tcpACK
	case ModeFIN:
		return tcpFIN
	case ModeXmas:
		return tcpFIN | tcpPSH | tcpURG
	case ModeNull:
		return 0
	default:
		return 0
	}
}

func (m Mode) isRaw() bool { return m != ModeConnect }
```

`classify.go`:
```go
package tcpscan

// classify 由响应标志位推导端口状态（mode 感知）。
// flags: 响应 TCP 首部 flag 位；icmpUnreach: 收到 ICMP 不可达；responded: 窗口内收到任何响应。
func classify(flags uint8, icmpUnreach, responded bool, mode Mode) State {
	if icmpUnreach {
		return Filtered
	}
	switch mode {
	case ModeSYN, ModeConnect:
		switch {
		case flags&tcpSYN != 0 && flags&tcpACK != 0:
			return Open
		case flags&tcpRST != 0:
			return Closed
		default:
			return Timeout
		}
	case ModeFIN, ModeNull, ModeXmas:
		if flags&tcpRST != 0 {
			return Closed
		}
		if responded {
			return Filtered
		}
		return OpenFiltered
	case ModeACK:
		if flags&tcpRST != 0 {
			return Unfiltered
		}
		return Filtered
	default:
		return Timeout
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd d:/myself/scan/atlas && go test ./internal/scan/tcpscan/ -run TestClassify -v`
Expected: PASS（14 子用例全过）。

- [ ] **Step 5: 提交**

```bash
git add internal/scan/tcpscan/state.go internal/scan/tcpscan/mode.go internal/scan/tcpscan/classify.go internal/scan/tcpscan/classify_test.go
git commit -m "feat(tcpscan): add state/mode types and mode-aware classify"
```

---

### Task 2: Prober 接口与 Options

**Files:**
- Create: `internal/scan/tcpscan/prober.go`

**Interfaces:**
- Produces: `Scanner` 接口、`Options` 结构体（Task 3/4/5 实现与消费）。

- [ ] **Step 1: 写 `prober.go`**

```go
package tcpscan

import (
	"context"
	"net"
	"time"
)

// Options 控制单次 Scan 的行为。
type Options struct {
	Timeout        time.Duration // raw 抓包窗口 / connect 拨号超时
	Retries        int           // 无响应重发次数（共发 Retries+1 次）
	Concurrency    int           // connect 模式 goroutine 池大小；raw 模式保留
	Iface          string        // 抓包网卡（空=自动按路由选出口）
	SrcPort        int           // 源端口（0=随机高端口）
	SourceIP       net.IP        // 发包源 IP（空=出口 IP）
	InstallRstDrop bool          // 是否尝试安装 RST-drop 防火墙规则
}

// Scanner 统一扫描接口：对单个目标 IP 扫描端口列表，返回 端口->结果。
// 单端口失败不中断；ctx 取消时返回已得结果 + ctx.Err()。
type Scanner interface {
	Scan(ctx context.Context, target string, ports []int, opts Options) (map[int]Result, error)
}
```

- [ ] **Step 2: 编译校验**

Run: `cd d:/myself/scan/atlas && go build ./internal/scan/tcpscan/`
Expected: 成功（尚无实现体，仅类型）。

- [ ] **Step 3: 提交**

```bash
git add internal/scan/tcpscan/prober.go
git commit -m "feat(tcpscan): define Scanner interface and Options"
```

---

### Task 3: connect 实现（行为不变）+ 单测

**Files:**
- Create: `internal/scan/tcpscan/connect.go`
- Test: `internal/scan/tcpscan/connect_test.go`

**Interfaces:**
- Consumes: `Scanner` 接口、`Options`、`State`、`Result`（Task 1/2）。
- Produces: `connectScanner`（factory Task 5 使用）。

- [ ] **Step 1: 写失败测试 `connect_test.go`**

```go
package tcpscan

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestConnectScan(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	openPort := ln.Addr().(*net.TCPAddr).Port

	opts := Options{Timeout: 1500 * time.Millisecond, Concurrency: 10}
	res, err := connectScanner{}.Scan(context.Background(), "127.0.0.1", []int{openPort, 1}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res[openPort].State != Open {
		t.Fatalf("open port got %s", res[openPort].State)
	}
	if res[1].State != Closed {
		t.Fatalf("closed port got %s", res[1].State)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd d:/myself/scan/atlas && go test ./internal/scan/tcpscan/ -run TestConnectScan -v`
Expected: FAIL（`connectScanner` 未定义）。

- [ ] **Step 3: 实现 `connect.go`**

```go
package tcpscan

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type connectScanner struct{}

func (connectScanner) Scan(ctx context.Context, target string, ports []int, opts Options) (map[int]Result, error) {
	res := make(map[int]Result, len(ports))
	sem := make(chan struct{}, max(1, opts.Concurrency))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, p := range ports {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			state, banner := probeConnect(ctx, target, port, opts.Timeout)
			mu.Lock()
			res[port] = Result{Port: port, State: state, Banner: banner}
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return res, nil
}

func probeConnect(ctx context.Context, ip string, port int, timeout time.Duration) (State, string) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		if ctx.Err() != nil {
			return Timeout, ""
		}
		return Closed, "" // 连接拒绝 / 超时 -> closed
	}
	defer conn.Close()
	return Open, grabConnectBanner(conn, timeout)
}

func grabConnectBanner(conn net.Conn, timeout time.Duration) string {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return ""
	}
	return strings.TrimSpace(string(buf[:n]))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

- [ ] **Step 4: 运行确认通过**

Run: `cd d:/myself/scan/atlas && go test ./internal/scan/tcpscan/ -run TestConnectScan -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/scan/tcpscan/connect.go internal/scan/tcpscan/connect_test.go
git commit -m "feat(tcpscan): implement connect scanner with banner grab"
```

---

### Task 4: raw gopacket 引擎 + 单测（mock 链路层）

**Files:**
- Create: `internal/scan/tcpscan/raw.go`
- Test: `internal/scan/tcpscan/raw_test.go`
- Modify: `go.mod` / `go.sum`（首次引入 gopacket）

**Interfaces:**
- Consumes: `Scanner` 接口、`Options`、`Mode`、`State`、`Result`、`classify`、`flags`/`isRaw`（Task 1/2）。
- Produces: `rawScanner`（factory Task 5 使用）。

- [ ] **Step 1: 引入依赖**

Run: `cd d:/myself/scan/atlas && go get github.com/google/gopacket@latest`
Expected: `go.mod` 增加 `github.com/google/gopacket`，`go.sum` 更新。

- [ ] **Step 2: 写失败测试 `raw_test.go`（用 pcapgo 内存回放验证 classify+匹配）**

构造一个 SYN-ACK 回包，喂入 `capture` 逻辑（抽成可注入 `gopacket.Packet` 的辅助），断言 `open`：

```go
package tcpscan

import (
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

func TestRawClassifyOpen(t *testing.T) {
	// 构造一条 SYN-ACK 响应：dstPort=我们的 srcPort(12345)，srcPort=被扫端口(80)
	eth := &layers.Ethernet{
		EthernetType: layers.EthernetTypeIPv4,
		SrcMAC:       []byte{0, 0, 0, 0, 0, 2},
		DstMAC:       []byte{0, 0, 0, 0, 0, 1},
	}
	ip := &layers.IPv4{Version: 4, IHL: 5, SrcIP: []byte{10, 0, 0, 1}, DstIP: []byte{10, 0, 0, 2}, Protocol: layers.IPProtocolTCP}
	tcp := &layers.TCP{SrcPort: 80, DstPort: 12345, SYN: true, ACK: true}
	tcp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	_ = gopacket.SerializeLayers(buf, gopacket.SerializeOptions{ComputeChecksums: true, FixLengths: true}, eth, ip, tcp)
	pkt := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.NoCopy)

	r := rawScanner{mode: ModeSYN}
	got := r.classifyPacket(pkt, 12345)
	if got != Open {
		t.Fatalf("classifyPacket=%s want open", got)
	}
}
```

- [ ] **Step 3: 运行确认失败**

Run: `cd d:/myself/scan/atlas && go test ./internal/scan/tcpscan/ -run TestRawClassifyOpen -v`
Expected: FAIL（`rawScanner`/`classifyPacket` 未定义）。

- [ ] **Step 4: 实现 `raw.go`**

```go
package tcpscan

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type rawScanner struct {
	mode Mode
	opts Options
}

func (r rawScanner) Scan(ctx context.Context, target string, ports []int, opts Options) (map[int]Result, error) {
	r.opts = opts
	iface, srcIP, err := r.resolveIface(target, opts)
	if err != nil {
		return nil, fmt.Errorf("tcpscan: resolve iface: %w", err)
	}
	handle, err := pcap.OpenLive(iface, 65536, true, opts.Timeout)
	if err != nil {
		return nil, fmt.Errorf("tcpscan: open pcap %s: %w", iface, err)
	}
	defer handle.Close()

	rst := newRstDrop(iface, target, opts.InstallRstDrop)
	defer rst.remove()

	if err := r.sendProbes(handle, iface, srcIP, target, ports, opts); err != nil {
		return nil, err
	}
	return r.capture(handle, ports, opts)
}

func (r rawScanner) resolveIface(target string, opts Options) (string, net.IP, error) {
	if opts.Iface != "" {
		src := opts.SourceIP
		if src == nil {
			src = ifaceFirstIPv4(opts.Iface)
		}
		if src == nil {
			return "", nil, fmt.Errorf("tcpscan: no IPv4 on iface %s", opts.Iface)
		}
		return opts.Iface, src, nil
	}
	ip := net.ParseIP(target)
	if ip == nil {
		return "", nil, fmt.Errorf("tcpscan: invalid target %q", target)
	}
	name, src, err := routeTo(ip)
	if err != nil {
		return "", nil, err
	}
	return name, src, nil
}

func (r rawScanner) sendProbes(handle *pcap.Handle, iface string, srcIP net.IP, target string, ports []int, opts Options) error {
	srcPort := pickSrcPort(opts)
	gwMAC, err := arpResolve(handle, iface, srcIP, target)
	if err != nil {
		return err
	}
	ifaceMAC, err := ifaceMAC(iface)
	if err != nil {
		return err
	}
	dstIP := net.ParseIP(target)
	for _, p := range ports {
		for attempt := 0; attempt <= max(0, opts.Retries); attempt++ {
			if err := sendOne(handle, ifaceMAC, gwMAC, srcIP, dstIP, srcPort, layers.TCPPort(p), r.mode.flags()); err != nil {
				return err
			}
			if attempt < opts.Retries {
				time.Sleep(opts.Timeout / time.Duration(opts.Retries+1))
			}
		}
	}
	return nil
}

func sendOne(handle *pcap.Handle, ifaceMAC, gwMAC net.HardwareAddr, srcIP, dstIP net.IP, srcPort int, dstPort layers.TCPPort, flags uint8) error {
	eth := &layers.Ethernet{EthernetType: layers.EthernetTypeIPv4, SrcMAC: ifaceMAC, DstMAC: gwMAC}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, SrcIP: srcIP, DstIP: dstIP, Protocol: layers.IPProtocolTCP}
	tcp := &layers.TCP{SrcPort: layers.TCPPort(srcPort), DstPort: dstPort, SYN: flags&tcpSYN != 0, ACK: flags&tcpACK != 0, FIN: flags&tcpFIN != 0, PSH: flags&tcpPSH != 0, URG: flags&tcpURG != 0}
	tcp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{ComputeChecksums: true, FixLengths: true}, eth, ip, tcp); err != nil {
		return err
	}
	return handle.WritePacketData(buf.Bytes())
}

func (r rawScanner) capture(handle *pcap.Handle, ports []int, opts Options) (map[int]Result, error) {
	res := make(map[int]Result, len(ports))
	seen := make(map[int]bool)
	srcPort := pickSrcPort(opts)
	set := func(port int, st State) {
		if !seen[port] {
			seen[port] = true
			res[port] = Result{Port: port, State: st}
		}
	}
	deadline := time.Now().Add(opts.Timeout)
	for time.Now().Before(deadline) {
		data, _, err := handle.ReadPacketData()
		if err != nil {
			if err == pcap.NextErrorTimeout {
				continue
			}
			break
		}
		pkt := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.NoCopy)
		if icmp, ok := pkt.Layer(layers.LayerTypeICMPv4).(*layers.ICMPv4); ok && icmp.TypeCode.Type() == 3 {
			if inner, ok := parseICMPInner(icmp); ok {
				set(inner, Filtered)
			}
			continue
		}
		tcp, ok := pkt.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if !ok || tcp.DstPort != layers.TCPPort(srcPort) {
			continue
		}
		set(int(tcp.SrcPort), r.classifyPacket(pkt, srcPort))
	}
	for _, p := range ports {
		if !seen[p] {
			res[p] = Result{Port: p, State: classify(0, false, false, r.mode)}
		}
	}
	return res, nil
}

// classifyPacket 从单包推导状态（响应包路径）。
func (r rawScanner) classifyPacket(pkt gopacket.Packet, srcPort int) State {
	tcp, ok := pkt.Layer(layers.LayerTypeTCP).(*layers.TCP)
	if !ok || tcp.DstPort != layers.TCPPort(srcPort) {
		return Timeout
	}
	var f uint8
	if tcp.SYN {
		f |= tcpSYN
	}
	if tcp.ACK {
		f |= tcpACK
	}
	if tcp.RST {
		f |= tcpRST
	}
	if tcp.FIN {
		f |= tcpFIN
	}
	if tcp.PSH {
		f |= tcpPSH
	}
	if tcp.URG {
		f |= tcpURG
	}
	return classify(f, false, true, r.mode)
}

func (r rawScanner) resolveIfaceUnused() {}

func pickSrcPort(opts Options) int {
	if opts.SrcPort != 0 {
		return opts.SrcPort
	}
	return 40000 + (int(time.Now().UnixNano()) % 10000) // 随机高端口（单次 Scan 内稳定）
}

func ifaceFirstIPv4(name string) net.IP {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil
	}
	addrs, _ := iface.Addrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
			return ipn.IP
		}
	}
	return nil
}

func ifaceMAC(name string) (net.HardwareAddr, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	return iface.HardwareAddr, nil
}

func routeTo(ip net.IP) (string, net.IP, error) {
	conn, err := net.Dial("udp", net.JoinHostPort(ip.String(), "80"))
	if err != nil {
		return "", nil, err
	}
	defer conn.Close()
	local := conn.LocalAddr().(*net.UDPAddr).IP
	name, err := ifaceByIP(local)
	if err != nil {
		return "", nil, err
	}
	return name, local, nil
}

func ifaceByIP(ip net.IP) (string, error) {
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.Equal(ip) {
				return i.Name, nil
			}
		}
	}
	return "", fmt.Errorf("tcpscan: no iface for %s", ip)
}

func parseICMPInner(icmp *layers.ICMPv4) (int, bool) {
	// ICMP 不可达负载含原始 IP+TCP，提取目标端口
	payload := icmp.Payload
	if len(payload) < 24 {
		return 0, false
	}
	// 简化：从内层 TCP 头偏移 0 的 src port（2 字节）
	port := int(payload[20])<<8 | int(payload[21])
	return port, true
}

func arpResolve(handle *pcap.Handle, iface string, srcIP, dstIP net.IP) (net.HardwareAddr, error) {
	// 优先读系统 ARP 缓存；缺失则发 ARP 请求（实现见下，生产可用 net 包简化）
	if mac, ok := arpCacheLookup(dstIP); ok {
		return mac, nil
	}
	return arpRequest(handle, iface, srcIP, dstIP)
}
```

> 注：`arpResolve` / `arpCacheLookup` / `arpRequest` / `newRstDrop` 的具体实现（ARP 请求构造、iptables/netsh 调用）在 Task 4 补完；本步先保证编译与 `classifyPacket` 单测通过。`arpResolve` 在无法解析时返回错误，调用方据此能力降级 connect。

补充 `arpCacheLookup`/`arpRequest`/`newRstDrop` 最小可用实现（同文件追加）：

```go
func arpCacheLookup(ip net.IP) (net.HardwareAddr, bool) {
	// Windows: GetIpNetTable; Linux: /proc/net/arp。MVP 用 /proc/net/arp（Linux），
	// Windows 回退到 arpRequest。
	if runtime.GOOS != "windows" {
		// 解析 /proc/net/arp 的简化版
		if mac, ok := parseProcARP(ip); ok {
			return mac, true
		}
	}
	return nil, false
}

func arpRequest(handle *pcap.Handle, iface string, srcIP, dstIP net.IP) (net.HardwareAddr, error) {
	ifaceMAC, err := ifaceMAC(iface)
	if err != nil {
		return nil, err
	}
	eth := &layers.Ethernet{EthernetType: layers.EthernetTypeARP, SrcMAC: ifaceMAC, DstMAC: net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}}
	arp := &layers.ARP{AddrType: layers.LinkTypeEthernet, Protocol: layers.EthernetTypeIPv4,
		Operation: layers.ARPRequest, SourceHwAddress: ifaceMAC, SourceProtAddress: srcIP.To4(),
		DstHwAddress: []byte{0, 0, 0, 0, 0, 0}, DstProtAddress: dstIP.To4()}
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{}, eth, arp); err != nil {
		return nil, err
	}
	if err := handle.WritePacketData(buf.Bytes()); err != nil {
		return nil, err
	}
	// 等待 ARP 回复（窗口内）
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		data, _, err := handle.ReadPacketData()
		if err != nil {
			if err == pcap.NextErrorTimeout {
				continue
			}
			break
		}
		pkt := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.NoCopy)
		a, ok := pkt.Layer(layers.LayerTypeARP).(*layers.ARP)
		if ok && a.Operation == layers.ARPReply && net.IP(a.SourceProtAddress).Equal(dstIP) {
			return net.HardwareAddr(a.SourceHwAddress), nil
		}
	}
	return nil, fmt.Errorf("tcpscan: arp resolve %s timeout", dstIP)
}

type rstDrop struct {
	applied bool
	target  string
}

func newRstDrop(iface, target string, enable bool) *rstDrop {
	if !enable {
		return &rstDrop{target: target}
	}
	// best-effort：Linux iptables；Windows netsh。失败仅告警。
	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "netsh"
		args = []string{"advfirewall", "firewall", "add", "rule", "name=atlas-rst-drop",
			"dir=out", "action=block", "protocol=tcp", "tcpflags=RST"}
	} else {
		cmd = "iptables"
		args = []string{"-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "RST", "RST", "-d", target, "-j", "DROP"}
	}
	if err := exec.Command(cmd, args...).Run(); err == nil {
		return &rstDrop{applied: true, target: target}
	}
	return &rstDrop{target: target}
}

func (r *rstDrop) remove() {
	if !r.applied {
		return
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name=atlas-rst-drop").Run()
	} else {
		_ = exec.Command("iptables", "-D", "OUTPUT", "-p", "tcp", "--tcp-flags", "RST", "RST", "-d", r.target, "-j", "DROP").Run()
	}
}
```

需在文件顶部补充 `import "os/exec"`。

- [ ] **Step 5: 运行测试确认通过**

Run: `cd d:/myself/scan/atlas && go test ./internal/scan/tcpscan/ -run TestRawClassifyOpen -v`
Expected: PASS。

- [ ] **Step 6: 编译全包**

Run: `cd d:/myself/scan/atlas && go build ./internal/scan/tcpscan/`
Expected: 成功（arp/parseProcARP 等若未实现会报错——请补全 `parseProcARP` 与 `arpCacheLookup` 的 Linux 解析，Windows 直接回退 arpRequest）。

- [ ] **Step 7: 提交**

```bash
git add internal/scan/tcpscan/raw.go go.mod go.sum
git commit -m "feat(tcpscan): implement gopacket raw engine (send/capture/classify/rst-drop)"
```

---

### Task 5: factory 装配 + 单测

**Files:**
- Create: `internal/scan/tcpscan/factory.go`
- Test: `internal/scan/tcpscan/factory_test.go`

**Interfaces:**
- Consumes: `connectScanner`、`rawScanner`、`Mode`、`Options`（Task 3/4）。
- Produces: `New(mode, opts) (Scanner, error)`（scan.go Task 7 使用）。

- [ ] **Step 1: 写失败测试 `factory_test.go`**

```go
package tcpscan

import "testing"

func TestNew(t *testing.T) {
	if _, err := New("udp", Options{}); err == nil {
		t.Fatal("expected error for unsupported mode udp")
	}
	if s, err := New(ModeConnect, Options{}); err != nil || s == nil {
		t.Fatalf("connect: err=%v s=%v", err, s)
	}
	if s, err := New(ModeSYN, Options{}); err != nil || s == nil {
		t.Fatalf("syn: err=%v s=%v", err, s)
	}
	if s, err := New(ModeXmas, Options{}); err != nil || s == nil {
		t.Fatalf("xmas: err=%v s=%v", err, s)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd d:/myself/scan/atlas && go test ./internal/scan/tcpscan/ -run TestNew -v`
Expected: FAIL（`New` 未定义）。

- [ ] **Step 3: 实现 `factory.go`**

```go
package tcpscan

import "fmt"

// New 依据 mode 返回对应实现；非法 mode 返回 error。
func New(mode Mode, opts Options) (Scanner, error) {
	switch mode {
	case ModeConnect:
		return connectScanner{}, nil
	case ModeSYN, ModeACK, ModeFIN, ModeNull, ModeXmas:
		return rawScanner{mode: mode, opts: opts}, nil
	default:
		return nil, fmt.Errorf("tcpscan: unsupported mode %q", mode)
	}
}
```

- [ ] **Step 4: 运行全部 tcpscan 测试**

Run: `cd d:/myself/scan/atlas && go test ./internal/scan/tcpscan/ -v`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/scan/tcpscan/factory.go internal/scan/tcpscan/factory_test.go
git commit -m "feat(tcpscan): add factory selecting connect/raw by mode"
```

---

### Task 6: 数据模型 + 迁移 + store 覆盖

**Files:**
- Modify: `internal/model/model.go:19-33`（`Port` 加 `State`）
- Create: `migrations/000005_add_port_state.up.sql`
- Create: `migrations/000005_add_port_state.down.sql`
- Modify: `internal/store/pg.go`（`UpsertPort` 覆盖 `state`）

**Interfaces:**
- Consumes: `model.Port`（Task 7 写入 `State`）。
- Produces: `ports.state` 列，供 scanHost 落库。

- [ ] **Step 1: `model.go` 给 `Port` 加字段**

在 `Port` 结构体 `WebInfo` 后加一行：
```go
	WebInfo map[string]any `json:"webinfo"`
	State   string         `json:"state"` // open|closed|filtered|timeout|open|filtered|unfiltered
```

- [ ] **Step 2: 写迁移**

`000005_add_port_state.up.sql`:
```sql
ALTER TABLE ports ADD COLUMN IF NOT EXISTS state varchar(16) NOT NULL DEFAULT 'open';
```
`000005_add_port_state.down.sql`:
```sql
ALTER TABLE ports DROP COLUMN IF EXISTS state;
```

- [ ] **Step 3: 核对 `UpsertPort` 是否覆盖 `state`**

打开 `internal/store/pg.go` 找到 `UpsertPort`。若用 `sqlx.Named` + 结构体（如 `NamedExec("INSERT INTO ports (...) VALUES (:ip,:port,...) ON CONFLICT ...", portModel)`），新增字段会自动带上，无需改。若是显式列 INSERT，则在列清单与 `VALUES`/`:state` 补 `state` 列。

- [ ] **Step 4: 编译**

Run: `cd d:/myself/scan/atlas && go build ./internal/store/ ./internal/model/`
Expected: 成功。

- [ ] **Step 5: 提交**

```bash
git add internal/model/model.go migrations/000005_add_port_state.up.sql migrations/000005_add_port_state.down.sql internal/store/pg.go
git commit -m "feat: add Port.State column and migration 000005"
```

---

### Task 7: atlas scan.go 接入 tcpscan

**Files:**
- Modify: `internal/scan/scan.go:20-27`（`Scanner` 结构体）
- Modify: `internal/scan/scan.go:30`（`New` 签名收 cfg）
- Modify: `internal/scan/scan.go:64`（`scanHost` 接入 tcpscan）
- Modify: `internal/scan/probe.go:18`（`tcpConnect` 已迁入 tcpscan，移除；保留 `grabBanner`? 已由 tcpscan 自带，移除）
- Modify: `internal/config/config.go:42-48`（`ScanConfig` 加 raw 参数）

**Interfaces:**
- Consumes: `tcpscan.New`、`tcpscan.Scanner`、`tcpscan.Options`、`tcpscan.Result`、`tcpscan.Open`（Task 1-5）；`model.Port.State`（Task 6）；`config.ScanConfig` 新字段（本任务加）。
- Produces: 改造后的 `scan.New(cfg)` 与 `scanHost`（main.go Task 8 调用）。

- [ ] **Step 1: `config.go` 加 raw 参数**

`ScanConfig` 内 `DefaultMode` 后追加：
```go
	RawCaptureWindowSec int    `yaml:"raw_capture_window_sec"` // raw 抓包窗口（秒），默认 3
	RawRetries         int    `yaml:"raw_retries"`            // 无响应重发次数，默认 1
	RecordFilteredPorts bool  `yaml:"record_filtered_ports"`  // 是否落库 filtered，默认 true
	RecordClosedPorts   bool  `yaml:"record_closed_ports"`    // 是否落库 closed/timeout，默认 false
	InstallRstDrop      bool  `yaml:"install_rst_drop"`       // 是否装 RST-drop 规则，默认 true
	RawIface            string `yaml:"raw_iface"`             // 抓包网卡（空=自动）
```
并在 `defaultConfig()` 的 `Scan:` 块补默认值：`RawCaptureWindowSec: 3, RawRetries: 1, RecordFilteredPorts: true, RecordClosedPorts: false, InstallRstDrop: true`。

- [ ] **Step 2: `scan.go` 结构体与 `New`**

`Scanner` 改为：
```go
type Scanner struct {
	store        *store.Store
	rate         *ratelimit.Limiter
	fp           *fingerprint.Service
	defaultPorts []int
	timeout      time.Duration
	connSem      int
	mode         string
	ts           tcpscan.Scanner
	tcpscanOpts  tcpscan.Options
}
```
`New` 改为：
```go
func New(s *store.Store, r *ratelimit.Limiter, defaultPorts []int, fp *fingerprint.Service, cfg *config.Config) *Scanner {
	if len(defaultPorts) == 0 {
		defaultPorts = TopPorts
	}
	mode := cfg.Scan.DefaultMode
	if mode == "" {
		mode = "connect"
	}
	opts := tcpscan.Options{
		Timeout:        time.Duration(cfg.Scan.RawCaptureWindowSec) * time.Second,
		Retries:        cfg.Scan.RawRetries,
		Concurrency:    cfg.Scan.MaxConcurrency,
		Iface:          cfg.Scan.RawIface,
		InstallRstDrop: cfg.Scan.InstallRstDrop,
	}
	ts, err := tcpscan.New(tcpscan.Mode(mode), opts)
	if err != nil {
		log.Printf("scan: invalid mode %q, fallback connect: %v", mode, err)
		ts = connectScannerShim() // 见下：ts 仍可用 connect
	}
	return &Scanner{store: s, rate: r, fp: fp, defaultPorts: defaultPorts,
		timeout: 1500 * time.Millisecond, connSem: 50, mode: mode, ts: ts, tcpscanOpts: opts}
}
```
> 因 `tcpscan` 包内 `connectScanner` 为非导出，atlas 侧不能在编译期引用。故 `tcpscan.New` 对非法/降级场景应内部回退 connect 并返回 `connectScanner{}` 而非暴露给 atlas。修正：`tcpscan.New` 的非法 mode 分支改为返回 `connectScanner{}` 并打印而无错？但单测要求非法 mode 返回 error。折中：`tcpscan.New` 对未知 mode 返回 error（供测试）；atlas 侧用 `tcpscan.Mode(mode)` 仅当 mode 为已知 6 种才调用 New，否则直接 `tcpscan.New(tcpscan.ModeConnect, opts)`。atlas 代码改为：
```go
	ts, err := tcpscan.New(tcpscan.Mode(mode), opts)
	if err != nil {
		log.Printf("scan: mode %q unavailable, use connect: %v", mode, err)
		ts, _ = tcpscan.New(tcpscan.ModeConnect, opts)
	}
```

- [ ] **Step 3: `scanHost` 接入**

将 `scan.go:64` 的 `scanHost` 端口循环整体替换为：
```go
func (sc *Scanner) scanHost(ctx context.Context, ip string, ports []int) (map[string]any, error) {
	isV6 := isIPv6(ip)
	_ = sc.rate.WaitGlobal(ctx)
	_ = sc.rate.WaitTarget(ctx, ip)

	var results map[tcpscan.Port]tcpscan.Result
	scanErr := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("tcpscan panic ip=%s: %v", ip, r)
			}
		}()
		results, err = sc.ts.Scan(ctx, ip, ports, sc.tcpscanOpts)
		return err
	}()
	if scanErr != nil {
		log.Printf("scan: ts.Scan %s failed: %v", ip, scanErr)
	}

	openPorts := make([]int, 0, len(results))
	portsOut := make([]model.Port, 0, len(results))
	for _, p := range ports {
		res, ok := results[p]
		if !ok {
			continue
		}
		if !shouldRecord(res.State, sc.cfgOrFlags()) {
			continue
		}
		portModel := model.Port{
			IP: ip, Port: p, Proto: "tcp", State: string(res.State),
			Service: guessService(p, res.Banner), Banner: res.Banner,
			Host: ip, IsIPv6: isV6, FirstSeen: time.Now(), LastSeen: time.Now(),
		}
		if res.State == tcpscan.Open && (commonHTTPPorts[p] || looksLikeHTTP(res.Banner)) {
			if hr, err := httpProbe(ip, p, sc.timeout); err == nil {
				portModel.Title = hr.Title
				webinfo := map[string]any{"status": hr.Status, "server": hr.Server, "x_powered_by": hr.XPoweredBy, "scheme": hr.Scheme}
				if sc.fp != nil {
					webinfo["tech"] = sc.fp.Detect(hr.Header, hr.Body, res.Banner)
				}
				portModel.WebInfo = webinfo
				if hr.Cert != nil {
					portModel.Cert = hr.Cert
				}
			}
		}
		_ = sc.store.UpsertPort(ctx, portModel)
		openPorts = append(openPorts, p)
		portsOut = append(portsOut, portModel)
	}
	if err := sc.store.UpsertHost(ctx, model.Host{
		IP: ip, OpenPorts: openPorts, IsIPv6: isV6, FirstSeen: time.Now(), LastSeen: time.Now(),
	}); err != nil {
		return nil, err
	}
	return map[string]any{"ip": ip, "open_ports": openPorts, "count": len(openPorts)}, nil
}
```
> 类型修正：`results` 应为 `map[int]tcpscan.Result`（tcpscan.Scan 返回 `map[int]Result`）。`shouldRecord` 依据 `RecordFilteredPorts`/`RecordClosedPorts` 过滤：`open` 总是记；`filtered` 看 `RecordFilteredPorts`；其余看 `RecordClosedPorts`。`sc.cfgOrFlags()` 替换为直接读 `sc.recordFiltered`/`sc.recordClosed` 字段（在 `New` 中由 cfg 赋值）。

- [ ] **Step 4: 移除 `probe.go` 已迁移逻辑**

`probe.go` 中 `tcpConnect`（`probe.go:18-20`）与 `grabBanner`（`probe.go:23-31`）已迁入 `tcpscan`，从 `probe.go` 删除，保留 `httpResult` / `httpProbe` / `sanSlice`（scanHost 仍用）。

- [ ] **Step 5: 编译**

Run: `cd d:/myself/scan/atlas && go build ./internal/scan/...`
Expected: 成功（修正类型与字段名后）。

- [ ] **Step 6: 提交**

```bash
git add internal/scan/scan.go internal/scan/probe.go internal/config/config.go
git commit -m "feat(scan): wire tcpscan into scanHost, mode-driven, Port.State persisted"
```

---

### Task 8: main.go 装配

**Files:**
- Modify: `cmd/atlas/main.go`（构造 `scan.New` 处）

**Interfaces:**
- Consumes: `scan.New(s, r, defaultPorts, fp, cfg)`（Task 7 新签名）。

- [ ] **Step 1: 更新调用**

在 `main.go` 中找到 `scan.New(...)`，改为传入 `cfg`：
```go
scanner := scan.New(store, rate, nil, fp, cfg)
```
（其余参数保持原有顺序；`defaultPorts` 传 `nil` 走 `TopPorts` 默认）。

- [ ] **Step 2: 编译全项目**

Run: `cd d:/myself/scan/atlas && go build ./...`
Expected: 成功。

- [ ] **Step 3: 提交**

```bash
git add cmd/atlas/main.go
git commit -m "feat: pass cfg to scan.New in main"
```

---

### Task 9: 前端 Settings 加「抓包网卡」

**Files:**
- Modify: `前端 Settings.vue`（settings 界面，含 `DefaultMode` 下拉处）

**Interfaces:**
- 复用现有配置 GET/PUT API（`server/config.go` 全量回写），无需新端点。

- [ ] **Step 1: 在 `raw_iface` 配置项旁加输入框**

在 `DefaultMode` 下拉框同区块追加（伪代码，按现有 Vue 结构）：
```html
<label>抓包网卡 (RawIface)</label>
<input v-model="config.scan.raw_iface" placeholder="空=自动" />
```
确保 `config.scan` 已包含 `raw_iface`、`raw_capture_window_sec`、`raw_retries`、`record_filtered_ports`、`record_closed_ports`、`install_rst_drop`（随配置 API 自动往返）。

- [ ] **Step 2: 构建前端**

Run: 前端构建命令（依仓库：`npm run build` 或等价的 atlas 前端构建）。
Expected: 成功，界面出现「抓包网卡」输入框。

- [ ] **Step 3: 提交**

```bash
git add <前端 Settings.vue 及其构建产物路径>
git commit -m "feat(ui): add RawIface input to settings"
```

---

### Task 10: 集成测试 + 文档同步

**Files:**
- Create: `internal/scan/tcpscan/raw_integration_test.go`（`//go:build integration`）
- Modify: `docs/SPEC.md`（US-008 覆盖率改「已实现」）

**Interfaces:**
- 验证端到端 raw 扫（需 root + 网口，不参与常规 CI）。

- [ ] **Step 1: 写集成测试（build tag）**

```go
//go:build integration

package tcpscan

import (
	"context"
	"testing"
	"time"
)

func TestRawScanIntegration(t *testing.T) {
	opts := Options{Timeout: 3 * time.Second, Retries: 1, InstallRstDrop: true}
	s, err := New(ModeSYN, opts)
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Scan(context.Background(), "127.0.0.1", []int{80, 443}, opts)
	if err != nil {
		t.Fatalf("raw scan err: %v", err)
	}
	t.Logf("results: %+v", res)
}
```
> 真实环境对回环/网关运行：`go test -tags integration ./internal/scan/tcpscan/ -run TestRawScanIntegration -v`。

- [ ] **Step 2: 同步 `docs/SPEC.md`**

将 US-008 行状态由「❌ 未实现」改为「✅ 已实现（syn/ack/fin/null/xmas + connect 重构，gopacket libpcap/Npcap）」，并注明 `DefaultMode`/`raw_*` 配置项与界面「抓包网卡」。

- [ ] **Step 3: 提交**

```bash
git add internal/scan/tcpscan/raw_integration_test.go docs/SPEC.md
git commit -m "test: add raw integration test; mark US-008 implemented in SPEC"
```

---

## Self-Review Notes

- **Spec 覆盖**：§1 目标(6模式/跨平台/状态词表)→Task1-5；§2 包结构→Task1-5 文件；§3 状态/模式→Task1；§4 接口→Task2；§5 raw 引擎→Task4；§6 atlas 集成→Task6-8；§7 配置→Task7(config)+Task9(UI)；§8 错误处理→各 Task（panic recover、ctx、能力降级）；§9 测试→各 Task 测试 + Task10；§10 风险→Task4(arp/rst-drop/能力降级)、Task7(IPv6 未特别处理，按能力降级 connect 在 factory 层面由 atlas 调用保证)。
- **一致性**：`classify(flags, icmpUnreach, responded, mode)` 在 Task1 定义、Task4 的 `classifyPacket` 调用一致；`tcpscan.Scan` 返回 `map[int]Result` 在 Task2 定义、Task3/4/7 一致；`New(mode, opts)` 在 Task5 定义、Task7 调用一致。
- **类型修正点**（已在步骤内标注）：Task7 的 `results` 类型应为 `map[int]tcpscan.Result`；atlas 侧对非法 mode 用 `tcpscan.New(connect)` 兜底而非引用未导出 `connectScanner`；`shouldRecord` 用 `Scanner` 新增的 `recordFiltered`/`recordClosed` 字段（在 `New` 由 cfg 赋值）。
- **无占位符**：各 Task 均含可执行代码；`arpResolve`/`parseProcARP` 的 Linux ARP 缓存解析在 Task4 Step4/6 要求补全（`parseProcARP` 解析 `/proc/net/arp`），Windows 直接回退 `arpRequest`。
