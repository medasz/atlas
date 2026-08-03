# 系统运行日志与链路追踪增强设计方案

- 日期：2026-08-03
- 状态：已评审通过
- 目标：引入统一的终端结构化日志输出架构（基于 Go 官方 `slog`），全链路覆盖系统启动、任务调度、端口发现、HTTP/指纹识别及批量 Bulk 落库过程，彻底提升排错与运行诊断效率。

## 1. 背景与需求

当前系统在扫描发包、任务调度、HTTP/指纹识别和写库落盘过程中缺少系统化的运行日志，排查任务停滞、网络丢包或扫描异常时极其困难。本方案旨在构建轻量、高性能、带终端高亮与结构化 Key-Value 属性的运行日志体系。

## 2. 详细设计

### 2.1 日志核心包架构 (`internal/logger`)

在 `internal/logger/logger.go` 中封装基于 `log/slog` 的结构化日志组件：
- **级别控制**：支持 `DEBUG`, `INFO`, `WARN`, `ERROR` 四级。
- **终端高亮**：根据 Level 自动附加 ANSI 终端颜色前缀（`[DEBUG]` 灰蓝, `[INFO]` 青绿, `[WARN]` 暖黄, `[ERROR]` 玫瑰红）。
- **结构化输出**：日志内容由固定消息与键值对参数（`slog.Attr`）组成。

### 2.2 全链路埋点规范

1. **cmd/atlas (服务启动)**
   - `INFO`: 系统启动、配置加载、连接池/ES 初始化状态。
2. **internal/task (任务调度)**
   - `INFO`: `[TaskCreated]` 任务解析与网段打散完成。
   - `INFO`: `[TaskStarted]` / `[TaskCompleted]` 任务生命周期与耗时统计。
3. **internal/scan (扫描引擎发包)**
   - `DEBUG`: `[ScanTarget]` 开始探测单个 IP 与端口列表。
   - `INFO`: `[PortFound]` 成功发现 Open 端口 `target=IP:Port`, `banner=xxx`。
   - `WARN`: 协议降级（如 IPv6 降级 `connect`）与 ARP 异常。
4. **internal/scan/pipeline.go (异步 Pipeline & Enricher)**
   - `DEBUG`: `[HttpProbe]` HTTP(S) 探测结果 (URL, 状态码, 标题, 提取的指纹)。
   - `INFO`: `[BatchFlush]` 批量 Bulk 写入 ES/PG 资产数及批次耗时。

## 3. 影响文件清单

- **`internal/logger/logger.go`** [NEW]: 结构化日志封装。
- **`cmd/atlas/main.go`**: 接入全局日志初始化。
- **`internal/task/task.go`**: 注入任务调度日志。
- **`internal/scan/scan.go`**: 注入端口扫描日志。
- **`internal/scan/pipeline.go`**: 注入 Pipeline 探测与批量写库日志。
