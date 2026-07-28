# Atlas 技术规范（SPEC）

## TCP 多模式扫描

统一接口 `tcpscan.Scanner.Scan(ctx, target, ports, opts) (map[int]Result, error)`，
按模式切换 6 种 TCP 扫描技术（参考 nmap TCP 扫描）：

| 模式 | 类型 | 发包 TCP flags | 可判定状态 | 实现 |
|---|---|---|---|---|
| `connect` | 全连接 | OS TCP 栈（`net.DialTimeout`） | open / closed | `connect.go` |
| `syn` | raw | SYN | open / closed / filtered / timeout | `raw.go` |
| `ack` | raw | ACK | unfiltered / filtered | `raw.go` |
| `fin` | raw | FIN | closed / open\|filtered | `raw.go` |
| `null` | raw | 无 | closed / open\|filtered | `raw.go` |
| `xmas` | raw | FIN+PSH+URG | closed / open\|filtered | `raw.go` |

### 状态词表（严格 nmap 语义）

`open` / `closed` / `filtered` / `timeout` / `open|filtered` / `unfiltered`

- SYN / connect：能区分 open / closed / filtered（timeout = 静默丢弃）。
- FIN / Null / Xmas：RFC 793 规定开放端口忽略非常规探测，故无响应归为 `open|filtered`（无法区分开放或被过滤）；收到 RST 为 `closed`；ICMP 不可达为 `filtered`。
- ACK：本就不探开放端口，仅探防火墙规则——收到 RST 为 `unfiltered`（端口可达），无响应/ICMP 为 `filtered`。

### 架构

- 纯逻辑包 `internal/scan/tcpscan`，不依赖 atlas store，可独立单测。
- `classify(flags, icmpUnreach, mode)` 为纯函数，无特权即可单测（14+ 用例）。
- raw 引擎（方案 A）：gopacket 构造 IPv4+TCP 帧，整块端口广发 + 单抓包窗口回收；
  单/小池源端口 + 按响应 `SrcPort==被扫端口` 会话匹配；RST-drop 规则规避内核回 RST（best-effort）。
- 跨平台：Linux=libpcap / Windows=Npcap，均走 gopacket raw；**仅能力降级**（抓包句柄打不开 → 降级 connect），不按 OS 降级。

### 构建标签

- 默认构建：raw 抓包代码由 `//go:build !raw_capture` 的 stub 提供，raw 模式一律返回
  `errRawUnavailable`，由 `scan.go` 降级为 connect。保证无 Npcap SDK 的开发机可编译。
- 生产镜像 / Windows（装 Npcap SDK）：`-tags raw_capture` 链接 `gopacket/pcap` 启用真实抓包。
- 集成测试：`go test -tags integration ./internal/scan/tcpscan/`（raw 还需 `-tags raw_capture` + 特权）。

### 持久化策略

`model.Port.State` 新增列（`migrations/000005_add_port_state`）。落库规则（`scan.shouldPersist`）：

- `open`：始终落库（确认的开放端口）。
- `filtered` / `open|filtered` / `unfiltered`：受 `ScanConfig.RecordFilteredPorts`（默认 true）控制，
  即「不确定 / 拓扑类」结果默认落库，closed/timeout 不在此列。
- `closed` / `timeout`：受 `ScanConfig.RecordClosedPorts`（默认 false）控制，防全端口扫描 PG 膨胀。

### 配置项（`config.ScanConfig`，界面 Settings 可编辑）

| 项 | 默认 | 含义 |
|---|---|---|
| `default_mode` | `connect` | 新建任务预填的扫描模式 |
| `raw_capture_window_sec` | 3 | raw 抓包窗口（秒） |
| `raw_retries` | 1 | 无响应重发次数 |
| `record_filtered_ports` | true | 是否落库 filtered |
| `record_closed_ports` | false | 是否落库 closed/timeout |
| `install_rst_drop` | true | 是否尝试安装 RST-drop 规则 |
| `raw_iface` | "" | 抓包网卡（空=自动选出口） |

### 已知限制

- IPv6：raw 抓包/ICMPv6 判定不同，本期 IPv6 强制降级 connect + 记日志。
- 运行时在界面修改 `default_mode` / `raw_iface` **无需重启即对新建任务生效**：配置 API 通过
  `Scanner.SetScanConfig` 推送，扫描器持有独立配置副本（`sync.RWMutex` 保护），`scanHost` 执行时实时读取。
  配置同时持久化到 YAML 文件（`config.Save`）；注意当前配置存储后端为 YAML 文件而非数据库。
- 单 src port 下若未安装 RST-drop 规则，内核可能对该端口回 RST（仅损失 stealth，不影响正确性）。
