# 端口块粒度进度设计（Spec）

- 日期：2026-07-27
- 状态：已与用户确认设计，待评审后进入实现计划
- 关联问题：扫描 `1-65535` 全端口时进度条长时间卡在 0%（上一轮已定位根因为「进度粒度是 IP 级而非端口级」+ 单 IP 限速 10/s 导致耗时数小时）

## 1. 目标与范围

### 目标
大端口范围（如 `1-65535`）扫描时，进度条从「0% → 100%」平滑推进，用户能实时看到进度在涨，不再长时间卡 0。

### 范围
- 仅改任务调度 / 存储 / 队列消息载体 / TaskItem 粒度。
- **不碰** 扫描引擎内部的 `scanHost` 并发（`connSem`）与限流（`WaitGlobal` / `WaitTarget`）逻辑——扫描速度不变，只让「进度可见性」变细。
- 不引入端口级断点续扫（用户选择「只要进度平滑」）。
- 不调整限流速率（提速是独立议题）。

### 非目标 / 明确不做
- 不改变 `WaitTarget(ip)` 的 10/s 单 IP 限流语义。
- 不对域名做端口切块（域名只扫少量 HTTP 端口，保持一个域名 = 一条 item）。

## 2. 核心思路

把「一个 IP = 一条 TaskItem」改为「一个 IP 的每一个**端口块** = 一条 TaskItem」。块大小由配置 `port_chunk_size` 控制，默认 1000。

- 65535 端口 ÷ 1000 ≈ **66 块** → 进度条出现 66 个台阶平滑上涨。
- 每块扫描完成调用一次 `MarkItemDone` → `CountTaskItems` 统计行数 → `Progress.done` 递增。
- 域名目标端口极少，保持「一个域名 = 一条 item」，其 `ports` 字段留空字符串。

## 3. 改动清单（按模块边界）

### 3.1 config（`internal/config/config.go` + `configs/atlas.yaml`）
- `ScanConfig` 新增字段：
  ```go
  PortChunkSize int `yaml:"port_chunk_size"` // 单 IP 端口切块大小，默认 1000
  ```
- `defaultConfig()` 的 `Scan` 段加 `PortChunkSize: 1000`。
- `configs/atlas.yaml` 的 `scan:` 段加 `port_chunk_size: 1000`。

### 3.2 数据库迁移（`migrations/000004_port_chunk.up.sql`）
```sql
-- 任务子项支持端口块粒度（断点续扫单元细化到端口块）
ALTER TABLE task_items ADD COLUMN IF NOT EXISTS ports TEXT NOT NULL DEFAULT '';
ALTER TABLE task_items DROP CONSTRAINT IF EXISTS task_items_task_id_target_key;
ALTER TABLE task_items
  ADD CONSTRAINT task_items_task_id_target_ports_key UNIQUE (task_id, target, ports);
```
- 旧数据兼容性：`ports` 列 `DEFAULT ''`，旧 item 变为 `(task_id, target, '')`，仍唯一，不冲突。
- 索引 `idx_items_task_status (task_id, status)` 仍有效，无需改动。

### 3.3 model（`internal/model/model.go`）
`TaskItem` 新增字段：
```go
type TaskItem struct {
	TaskID string         `json:"task_id"`
	Target string         `json:"target"`
	Ports  string         `json:"ports"`   // 端口块规格，如 "1-1000"；域名/空块为 ""
	Status int            `json:"status"`  // 0 pending 1 done 2 filtered
	Result map[string]any `json:"result"`
}
```

### 3.4 store（`internal/store/pg.go`）
- `UpsertTaskItem`：SQL 增加 `ports` 列，冲突键改为 `(task_id, target, ports)`。
  ```go
  INSERT INTO task_items (task_id, target, ports, status, result)
  VALUES ($1,$2,$3,$4,$5)
  ON CONFLICT (task_id, target, ports) DO UPDATE SET status=EXCLUDED.status, result=EXCLUDED.result
  ```
- `ListTaskItems`：`SELECT` 增加 `ports` 列并 `Scan` 到 `it.Ports`。注意 `Scan` 字段顺序要同步增加（当前是 `task_id, target, status, result` → 改为 `task_id, target, ports, status, result`）。
- `MarkItemDone`：签名增加 `ports` 参数，内部构造 `TaskItem{TaskID, Target, Ports: ports, Status: done, Result}`。
  ```go
  func (s *Store) MarkItemDone(ctx context.Context, taskID, target, ports string, result map[string]any) error
  ```
- `CountTaskItems`：不变（统计行数即可，块数即行数）。

### 3.5 queue（`internal/queue/nats.go`）
`TaskMsg` 增加 `Ports` 字段（JSON 自动序列化）：
```go
type TaskMsg struct {
	TaskID string `json:"task_id"`
	Target string `json:"target"`
	Ports  string `json:"ports"`
	Kind   string `json:"kind"`
}
```

### 3.6 task 服务（`internal/task/task.go`）
- `Service` 结构体新增字段 `defaultPorts []int` 与 `portChunkSize int`；`New` 签名增加这两个参数，`main.go` 传入。
- **Processor 接口变更**：
  ```go
  type Processor interface {
  	Process(ctx context.Context, task model.Task, target, ports string) (map[string]any, error)
  }
  ```
  - `noopProcessor.Process` 同步改签名（忽略 `ports`）。
  - `scan.Process`：若 `ports != ""` → `ParsePortSpec(ports)` 得到端口列表；否则回退现有 `portsFor(task)`。
  - `vuln.Engine.Process`：同步改签名为 `Process(ctx, task, target, ports string)`，内部忽略 `ports`（仍用自身固定 HTTP 端口集合）。
- **Create**：
  1. `targets, _ := scope.Expand(sc)` 不变。
  2. 新增辅助：对 IP target 解析端口列表 `plist` = `scopePorts`（若 `sc["ports"]` 是合法串）否则 `svc.defaultPorts`；对域名 target `plist = nil`（不切块）。
  3. 对每个 target：
     - 若是 IP 且 `len(plist) > 0`：按 `portChunkSize` 切片，得到若干块（每块 `"lo-hi"`）；每块建一条 `TaskItem{Target: ip, Ports: chunk, Status: pending/filtered}`。
     - 否则（域名或无端口）：建一条 `TaskItem{Target: t, Ports: "", Status: ...}`。
  4. `Progress = {total: 总块数, done: 0}`；`total` 即所有 target 的块数之和。
  5. 黑名单过滤：按 IP/域名整体过滤（块级 item 继承同一 target 的过滤状态）。
- **dispatch**：发布 `TaskMsg{TaskID, Target: it.Target, Ports: it.Ports, Kind: subject}`；进程内分支同样传 `it.Ports`。
- **RegisterWorker handler**：`unmarshal` 后调用 `svc.processOne(ctx, task, msg.Target, msg.Ports)`。
- **processOne(ctx, task, target, ports string)**：
  - `proc.Process(ctx, task, target, ports)`
  - `MarkItemDone(ctx, task.ID, target, ports, res)`
  - 其余（WaitGlobal / CountTaskItems / UpdateTaskProgress / UpdateTaskStatus）不变。
- **runInProcess**：`processOne(ctx, task, it.Target, it.Ports)`。

### 3.7 main（`cmd/atlas/main.go`）
- `task.New(st, q, auditor, bl, limiter, cfg.Scan.MaxConcurrency, defaultPorts, cfg.Scan.PortChunkSize)`（按新 `New` 签名补参）。

## 4. 数据流（以 `1-65535` 对 127.0.0.1 为例）

```
Create:
  解析端口 [1..65535]，按 1000 切块 → 66 块
  建 66 条 task_items: (ip,"1-1000"),(ip,"1001-2000"),...,(ip,"65001-65535")
  Progress.total = 66, done = 0
  → 发布 66 条 NATS 消息，每条带 Ports="lo-hi"

atlas2 Worker（handler 同步，逐条消费）:
  processOne(ip, "1-1000")
    → scan.Process(ip,"1-1000") → scanHost(ports=[1..1000])
    → MarkItemDone(ip,"1-1000") → CountTaskItems → done=1
  进度 1/66
  processOne(ip,"1001-2000") → done=2
  ...
  processOne(ip,"65001-65535") → done=66 → UpdateTaskStatus(Done)
  进度 66/66
```

## 5. 兼容性与风险

- **旧数据**：`ports` 列默认 `''`，旧 item 仍唯一；`UpsertTaskItem` 新冲突键对旧数据无影响。
- **单实例顺序执行**：atlas2 的订阅 handler 同步调用 `processOne`，而 `processOne` 内 `scanHost` 的 `wg.Wait` 会阻塞，故同一 IP 的多个块**顺序执行**，进度按块递增；且不会并发打爆同一 IP 的 `WaitTarget` 限流。
- **速度不变**：总耗时与切块前一致（`WaitTarget(ip)` 恒为 10/s），但进度可见性从「2 态（0/1）」变为「66 态」。
- **data race**：切块后 `scan.Process` 优先用消息携带的 `ports`，不再读共享的 `task.Scope`（仅当 `ports==""` 才回退），降低多 goroutine 并发读 `task.Scope` 的竞态面。

## 6. 验证方式

- **单元测试**：`internal/scan/ports_test.go` 增加端口切块辅助函数测试（给定端口列表与块大小，验证切片数量与边界正确）。
- **集成验证**（本机 docker）：
  1. `docker-compose up --build -d` 重建。
  2. 通过 API 创建 `{"kind":"scan","scope":{"targets":["127.0.0.1"],"ports":"1-65535"}}` 任务。
  3. 轮询 `GET /api/tasks/{id}` 的 `progress.done/total`，确认 `done` 从 0 逐步上涨到 66，不再卡 0。
  4. 确认 `task_items` 表对该任务有 66 行、`ports` 列分别为 `1-1000`…`65001-65535`。
- **回归**：创建小范围任务（`1-100`、单一域名）确认仍 `0/1 → 1/1` 正常完成。

## 7. 不变量 / 验收标准

- 全端口任务进度条平滑递增（块数 ≥ 2 时中间态可见）。
- 既有「IP 级 / 域名级」任务行为不变（小范围仍 `1/1`）。
- 无 DB 唯一约束冲突、无 data race 回归。
- 编译通过、现有测试通过。
