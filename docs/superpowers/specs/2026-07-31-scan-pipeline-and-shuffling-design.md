# 扫描引擎高并发重构方案：三级流水线解耦与目标 IP 随机打散

- 日期：2026-07-31
- 状态：已评审通过
- 目标：将扫描引擎解耦为“纯端口发现”、“异步 HTTP/指纹探测”、“批量 Bulk 落库”三级流水线，并引入 IP Shuffling 随机打散，彻底消除阻塞与防火墙拦截丢包，实现十倍以上扫描吞吐量提升。

## 1. 背景与瓶颈

当前瓶颈：
1. **强耦合同步阻塞**：单 IP 端口扫描、HTTP/TLS 探测、ES/PG 落库是在单线程/Goroutine 中同步执行的。HTTP 请求超时（1.5s）或写库延迟会直接卡死端口扫描发包。
2. **端口并发被写死**：`connSem` 硬编码限制为 50，无论全局设置多少并发，单 IP 内部均被拆为多轮串行。
3. **连续发包触发防火墙 Drop**：顺序对单一 IP 连续发送 1000 个端口包易触发防火墙 rate limit 丢包，引发大量 1.5s 物理超时。

## 2. 架构重构设计

### 2.1 三级异步流水线 (3-Stage Pipeline Architecture)

```
[ Phase 1: 纯端口发现 (Port Discovery) ] 
       │ 纯 TCP 探测，只产生 {IP, Port, State, Banner} 事件
       ▼ (Chan enrichChan 容量 10000)
[ Phase 2: 异步指纹/HTTP 探测 (Async Enricher Pool) ] 
       │ 100+ Worker 池并发 HTTP 探测、Title/Cert/Tech 识别
       ▼ (Chan writerChan 容量 10000)
[ Phase 3: 批量缓冲落库 (Batch Bulk Writer) ] 
       │ 50 条或每 200ms 批量 Bulk 写入 ES & PG
```

1. **Phase 1 (Port Discovery)**:
   - 移除端口扫描中的同步 `httpEnrich` 与 `sc.asset.Upsert` 调用。
   - 探查到端口 `Open`（或符合 `shouldPersist` 规则）时，向 `enrichChan` 投递 `openPortEvent` 后继续以最高速率发包。
   - 提升内部并发度，由配置控制。

2. **Phase 2 (Async Enricher Pool)**:
   - 启动独立高并发 Worker 池（如 100 个 Goroutines）监听 `enrichChan`。
   - 仅对开放端口并发执行 `httpProbe`，提取 Web Title、Header、Cert 及指纹。
   - 构造完整的 `model.Asset` 投递至 `writerChan`。

3. **Phase 3 (Batch Bulk Writer)**:
   - 启动后台批量写入器，按 **50 条** 或 **每 200ms** 定时器合并批次。
   - 批量执行 ES 写入与 PG `ip_survivals` 打卡。

### 2.2 目标 IP 随机打散 (IP Shuffling)

- 在 `scope.Expand` 扩展 CIDR / IP 列表时，引入 Fisher-Yates 算法对 IP 序列进行随机打乱。
- 保证发包在时间与空间（目标网段）上均匀分布，避免对单一目标 IP 密集发包触发防护丢包。

## 3. 关联文件与接口

- **`internal/scope/scope.go`**: 新增 `ShuffleIPs([]string)` 方法。
- **`internal/scan/scan.go`**: 重构 `Scanner`，引入 `enrichChan`、`writerChan` 及异步 Worker 启动逻辑；重构 `scanHost` 与 `finishHost`。
- **`internal/scan/pipeline.go`**: 新增 `enrichWorker` 与 `batchWriter` 管理逻辑。
