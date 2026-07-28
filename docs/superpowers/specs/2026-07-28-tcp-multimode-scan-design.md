# TCP 多模式扫描 · 设计规格

- 日期：2026-07-28
- 范围：atlas 资产扫描平台 `internal/scan` 模块
- 关联：US-008（端口扫描多模式）、`docs/SPEC.md` 覆盖率矩阵第 11 节

## 1. 目标与范围

实现统一接口切换 6 种 TCP 扫描模式：`syn` / `connect` / `ack` / `fin` / `null` / `xmas`。

- `connect` 重构进统一 Prober 接口，**行为不变**（沿用 `net.DialTimeout`，当前实现位于 `internal/scan/probe.go:18` 的 `tcpConnect`）。
- 其余 5 种（syn/ack/fin/null/xmas）走 gopacket raw 引擎：Linux 用 `libpcap`，Windows 用 `Npcap`，**不按操作系统回退 connect**，仅在「抓包句柄打不开（缺权限/缺 Npcap）」时按能力降级 connect 并告警。
- 端口状态词表严格遵循 nmap 语义。
- raw 模式并发 = 并行目标 IP 数；单个目标 IP 内整块端口一次性发出 + 一个抓包窗口回收响应。

非目标（本期不做）：UDP 扫描、IPv6 raw（本期 IPv6 强制 connect + warn）、SCTP、协议栈指纹（OS 指纹）。

## 2. 包结构（与 store 解耦）

新增独立子包 `internal/scan/tcpscan`，**不依赖 atlas store / model**，便于单测与复用：

```
internal/scan/tcpscan/
  state.go      // State / Result 类型与分类常量
  mode.go       // Mode 类型 + flags()（各 raw 模式的 TCP flag 位）
  prober.go     // Scanner 接口 + Options
  connect.go    // connect 模式实现（封装 net.DialTimeout）
  raw.go        // gopacket 引擎 + classify + RST 规避
  factory.go    // New(mode, opts) 选择实现
  state_test.go // classify 纯函数单测
  raw_test.go   // 链路层回放 / mock 单测
```

`internal/scan/scan.go` 的 `Scanner` 持有 `tcpscan.Scanner` 实例，在 `scanHost` 中接入并将结果写入 `model.Port.State`。

## 3. 状态与模式定义

### 3.1 状态词表（严格 nmap 语义）

```go
package tcpscan

type State string

const (
	Open         State = "open"          // 端口在监听
	Closed       State = "closed"        // 可达但无服务（收到 RST）
	Filtered     State = "filtered"      // 防火墙显式拒绝 / 主机不可达（ICMP 不可达）
	Timeout      State = "timeout"       // 静默丢弃，重试后仍无响应
	OpenFiltered State = "open|filtered" // FIN/Null/Xmas：开放或被过滤，无法区分
	Unfiltered   State = "unfiltered"    // ACK：端口可达（防火墙放行）
)

type Result struct {
	Port   int
	State  State
	Banner string // 仅 open 且有 banner 时填充
}
```

### 3.2 扫描模式

```go
type Mode string

const (
	ModeSYN     Mode = "syn"
	ModeConnect Mode = "connect"
	ModeACK     Mode = "ack"
	ModeFIN     Mode = "fin"
	ModeNull    Mode = "null"
	ModeXmas    Mode = "xmas"
)

// TCP 首部 flag 位（与 gopacket layers.TCP 常量一致）
const (
	tcpFIN uint8 = 0x01
	tcpSYN uint8 = 0x02
	tcpRST uint8 = 0x04
	tcpPSH uint8 = 0x08
	tcpACK uint8 = 0x10
	tcpURG uint8 = 0x20
)

// flags 仅对 raw 模式有意义；connect 不走此路径。
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
```

## 4. 统一接口

```go
type Scanner interface {
	// Scan 对单个目标 IP 扫描给定端口列表，返回 端口->结果 映射。
	// 单个端口失败不中断整体；ctx 取消时返回已得结果 + ctx.Err()。
	Scan(ctx context.Context, target string, ports []int, opts Options) (map[int]Result, error)
}

type Options struct {
	Timeout     time.Duration // raw 抓包窗口基准 / connect 拨号超时
	Retries     int           // 无响应重发次数（默认 1，即共发 Retries+1 次）
	Concurrency int           // 并行目标 IP 数（raw 模式生效）；connect 模式为 goroutine 池大小
	Iface       string        // 抓包网卡名（raw 模式，空=自动选出口网卡）
	SrcPort     int           // 源端口（0=随机高端口）
	SourceIP    net.IP        // 发包源 IP（空=出口 IP）
}
```

工厂：

```go
// New 依据 mode 返回对应实现；mode 非法返回 error。
func New(mode Mode, opts Options) (Scanner, error)
```

## 5. raw 引擎机制（方案 A：gopacket + pcap）

### 5.1 抓包句柄
- `pcap.OpenLive(iface, snaplen, promisc, timeout)` 打开句柄。
- Linux 依赖 `libpcap`（构建镜像安装 `libpcap-dev`）；Windows 运行时依赖 `Npcap`（`gopacket/pcap` 自动使用）。
- 打开失败（无 `CAP_NET_RAW` / 无 Npcap）→ 返回可识别错误；调用方按**能力**降级 connect + 告警（非操作系统降级）。

### 5.2 发包
- 对块内每个端口构造 `layers.IPv4 + layers.TCP`，TCP flags 由 `mode.flags()` 决定，src port 使用单/小池高随机端口（默认随机选一个高端口复用整块）。
- 整批 `handle.WritePacketData` 发出；重试时按 `Timeout / (Retries+1)` 间隔补发同样的探测包。

### 5.3 抓包窗口
- 窗口时长 = `opts.Timeout`；循环 `handle.ReadPacketData` 直到窗口耗尽或所有端口已得响应。
- 解析 Ethernet→IPv4→TCP；若载荷为 ICMP 且 `Type==3`（Code 1/2/3/9/10 或 admin prohibited），记 `filtered`。

### 5.4 会话匹配
- 因整块共用同一 src port，响应均回到该 src port；以「响应 TCP 的 SrcPort == 被扫端口」做会话匹配，规避源端口耗尽（无需 65535 个源端口）。

### 5.5 分类（mode 感知）
纯函数 `classify(flags uint8, icmpUnreach bool, mode Mode) State`：

| 模式 | SYN-ACK | RST | ICMP 不可达 | 无响应(timeout) |
|---|---|---|---|---|
| syn / connect | open | closed | filtered | timeout |
| fin / null / xmas | — | closed | filtered | open\|filtered |
| ack | — | unfiltered | filtered | filtered |

> 注：connect 模式不走 raw，由 `net.DialTimeout` 成功/失败/超时直接映射为 open/closed/timeout，复用同一 `State` 词表。

### 5.6 RST 规避（best-effort）
- 尝试安装丢弃出站 RST 的规则，避免内核对本机未绑定 src port 回 RST，保持半开放语义与 stealth：
  - Linux：`iptables -A OUTPUT -p tcp --tcp-flags RST RST -d <targetCIDR> -j DROP`
  - Windows：`New-NetFirewallRule` / `netsh advfirewall` 等效出站 block 规则
- 扫描窗口结束（或 ctx 取消）后卸载规则。
- 安装失败仅告警，不影响正确性（pcap 在链路层已先捕获 SYN-ACK）。

### 5.7 并发
- 调用方（atlas `scanHost` 或其外层）对每个目标 IP 各起一个 `Scan`，由 `opts.Concurrency` 信号量约束并行 IP 数。
- 单个 IP 内部不并行发包（整块一次性发出，单一抓包窗口）。

## 6. atlas 集成

### 6.1 Scanner 改造（`internal/scan/scan.go`）
- `Scanner` 结构体（`scan.go:20-27`）新增字段：`mode string`、`ts tcpscan.Scanner`、`tcpscanOpts tcpscan.Options`。
- `New()`（`scan.go:30`，当前签名 `(s *store.Store, r *ratelimit.Limiter, defaultPorts []int, fp *fingerprint.Service)`）改为接收 `cfg *config.Config`，据 `cfg.Scan.DefaultMode` 与新增 raw 参数构建 `tcpscan.New(...)`。
- `scanHost`（`scan.go:64`）：保留逐端口 goroutine 与既有 panic 安全网（`scan.go:77` 的 `recover`），但端口探测改调 `ts.Scan(ctx, ip, plist, opts)` 得到 `map[int]tcpscan.Result`；按结果构造 `model.Port`，写入 `State = string(res.State)`；HTTP 门控（`scan.go:105`，`commonHTTPPorts[p] || looksLikeHTTP(banner)`）仅当 `res.State == tcpscan.Open` 触发。
- `scanHost` 外层对 `ts.Scan` 调用再加 `recover` 兜底，确保单目标 panic 不拖垮 worker。

### 6.2 数据模型（`internal/model/model.go`）
- `Port`（`model.go:19-33`）新增字段：`State string \`json:"state"\``。

### 6.3 迁移
- 新增 `migrations/000005_add_port_state.up.sql`：
  ```sql
  ALTER TABLE ports ADD COLUMN IF NOT EXISTS state varchar(16) NOT NULL DEFAULT 'open';
  ```
- 配套 `.down.sql` 删除该列。
- 确认 `store.UpsertPort`（`internal/store/pg.go`）覆盖新列（若使用 sqlx 具名结构体插入则自动带上；若为显式列需补一列）。

### 6.4 装配（`cmd/atlas/main.go`）
- `scan.New` 调用处传入 `cfg`（当前 `main.go` 构造 `Scanner` 的代码需更新签名）。

### 6.5 落库策略（默认）
- `open`：总是落库（现状行为）。
- `filtered`：受 `cfg.Scan.RecordFilteredPorts`（默认 true）控制 —— filtered 揭示防火墙拓扑，价值高。
- `closed` / `timeout`：受 `cfg.Scan.RecordClosedPorts`（默认 **false**）控制 —— 全端口扫会产生海量 closed，默认不落库避免 PG 膨胀；需要时开启。
- `open|filtered` / `unfiltered`：按 `RecordClosedPorts` 同档处理（默认不落库，因非确定开放）。

## 7. 配置项（`internal/config/config.go` ScanConfig）

| 项 | 默认 | 含义 |
|---|---|---|
| `DefaultMode` | `"connect"` | 已存在（`config.go:43`），本期真正生效并驱动分派 |
| `RawCaptureWindowSec` | 3 | raw 抓包窗口（秒） |
| `RawRetries` | 1 | 无响应重发次数 |
| `RecordFilteredPorts` | true | 是否落库 filtered |
| `RecordClosedPorts` | false | 是否落库 closed / timeout / open\|filtered / unfiltered |
| `InstallRstDrop` | true | 是否尝试安装 RST-drop 防火墙规则 |
| `RawIface` | "" | 抓包网卡（空=自动选出口）。**界面可编辑**：经 `server/config.go` 的全量配置读写 API 暴露，Settings 界面提供「抓包网卡」输入框，与 `DefaultMode` 同机制（运维无需改 yaml 即可切换） |

`DefaultPortRange` / `MaxConcurrency` / `PerTargetRPS` / `PortChunkSize` 沿用既有配置。

## 8. 错误处理

- 单端口失败不中断整扫：结果以 `map[int]Result` 返回，未决端口标 `timeout` 或 `filtered`，绝不 panic 外泄。
- `ctx.Done()` → 返回已得结果 + `ctx.Err()`。
- pcap 句柄打不开 → 返回可识别错误，调用方按能力降级 connect + warn（**非** OS 降级）。
- mode 非法 → `factory.New` 返回 error，上层（任务创建/扫描入口）拒绝任务。
- `scanHost` 既有 panic recover（`scan.go:77`）保留；`ts.Scan` 外层再加 recover 兜底。
- IPv6 目标（`isIPv6(ip)`）→ raw 引擎强制转 connect + warn（能力边界，非 OS 降级）。

## 9. 测试策略

- `classify` 纯函数单测：6 模式 ×（SYN-ACK / RST / ICMP / 无响应）全覆盖，无需 root。
- raw 引擎单测：将 pcap 句柄抽象为 `linkLayer` 接口，用 `pcapgo` 内存回放或构造包喂入，断言状态映射正确。
- connect 单测：本地监听端口 + 关闭端口对照 open/closed/timeout。
- 集成测试（`//go:build integration`，需 root + 网口）：对回环/网关真实 raw 扫，人工核对。
- 回归：connect 路径在 `RecordClosedPorts=false` 时输出与现状一致。

## 10. 风险与未决

- **Npcap 运行时依赖**：Windows 主机必须安装 Npcap，否则能力降级 connect（非代码回退）。
- **RST-drop 规则**：默认开启（`InstallRstDrop=true`），失败仅告警；属运维动作。
- **IPv6**：raw 抓包与 ICMPv6 判定不同于 IPv4，本期 IPv6 强制 connect + warn（能力边界）。
- **单 src port RST 风暴**：无 RST-drop 规则时内核对该端口回大量 RST，需压测确认不影响捕获时序。
- **store 覆盖**：`UpsertPort` 是否结构体具名插入待实测确认（§6.3）。

## 11. 实施顺序（建议）

1. `tcpscan` 包骨架：`state.go` / `mode.go` / `prober.go` / `factory.go` + `classify` 纯函数与单测。
2. `connect.go`：封装 `net.DialTimeout`，返回 `map[int]Result`（行为不变）。
3. `model.Port.State` + 迁移 `000005` + 确认 `UpsertPort` 覆盖。
4. `raw.go`：gopacket 收发 + 窗口抓包 + `classify` + RST 规避；`linkLayer` mock 单测。
5. `scan.go` 改造：`Scanner` 加字段、`New` 收 cfg、`scanHost` 接入 `tcpscan` + HTTP 门控按 open。
6. `cmd/atlas/main.go` 装配 + `config.go` 新增配置项。
7. **前端 Settings 界面**：在 `Settings.vue` 新增「抓包网卡 (RawIface)」输入框（默认空=自动），与已有 `DefaultMode` 下拉框并列；经现有配置 API 持久化（无需新增端点）。
8. 集成测试 + 压测 + 文档更新（`docs/SPEC.md` 覆盖率矩阵同步）。
