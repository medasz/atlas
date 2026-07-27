# 端口块粒度进度实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把任务子项粒度从「一个 IP = 一条 TaskItem」细化到「一个 IP 的每个端口块 = 一条 TaskItem」，使大端口范围扫描的进度条平滑递增，不再长时间卡 0。

**Architecture:** 在存储层为 `task_items` 增加 `ports` 列并将唯一约束改为 `(task_id, target, ports)`；队列消息 `TaskMsg` 与 `Processor.Process` 接口增加 `ports` 参数在全链路透传；`task.Create` 按可配置块大小（`port_chunk_size`，默认 1000）把端口列表切块建多条 item，进度按块数统计。扫描引擎内部并发/限流逻辑不动。

**Tech Stack:** Go 1.21+，`github.com/jackc/pgx/v5`（PostgreSQL），`github.com/nats-io/nats.go`，`gopkg.in/yaml.v3`，标准库 `testing`。

## Global Constraints

- 扫描引擎内部 `scanHost` 的 `connSem` 并发与 `WaitGlobal`/`WaitTarget` 限流逻辑**不得修改**（速度不变，仅进度可见性变细）。
- `Processor` 接口签名统一改为 `Process(ctx context.Context, task model.Task, target, ports string) (map[string]any, error)`；`scan`/`vuln`/`noop` 三处实现同步改签名。
- `ports` 块字符串格式必须能被 `scan.ParsePortSpec` 精确还原：连续递增块用 `"lo-hi"`，非连续块用逗号拼接（如 `"21,22,23,53,80"`）。
- 旧数据兼容：`ports` 列 `DEFAULT ''`，旧 item 变为 `(task_id, target, '')` 仍唯一。
- 配置项 `port_chunk_size` 默认 1000，位于 `scan:` 段。
- 源码改动后必须 `docker-compose up --build` 重建镜像才生效。

---

## File Structure

| 文件 | 职责 | 改动 |
|------|------|------|
| `internal/config/config.go` | 配置结构 | `ScanConfig` 加 `PortChunkSize`；`defaultConfig` 默认 1000 |
| `configs/atlas.yaml` | 运行配置 | `scan:` 加 `port_chunk_size: 1000` |
| `migrations/000004_port_chunk.up.sql` | 迁移上 | 加 `ports` 列 + 新唯一约束 |
| `migrations/000004_port_chunk.down.sql` | 迁移下 | 回滚 |
| `internal/model/model.go` | 数据模型 | `TaskItem` 加 `Ports` 字段 |
| `internal/store/pg.go` | 仓储 | `UpsertTaskItem`/`ListTaskItems`/`MarkItemDone` 适配 `ports` |
| `internal/queue/nats.go` | 队列消息 | `TaskMsg` 加 `Ports` |
| `internal/task/task.go` | 调度核心 | `New`/`Processor`/`Create`/`dispatch`/`processOne`/`handler`/`runInProcess` 全链路透传 `ports` + 引入 `chunkSpec` 与 `portsForScope` |
| `internal/task/task_test.go` | 单测 | 测 `chunkSpec` 切块正确性 |
| `internal/scan/scan.go` | 扫描引擎 | `Process` 签名加 `ports`，优先用消息端口 |
| `internal/vuln/engine.go` | 漏洞引擎 | `Process` 签名加 `ports`（忽略） |
| `cmd/atlas/main.go` | 启动装配 | `task.New` 补 `defaultPorts`、`portChunkSize` |

---

### Task 1: 配置项 `port_chunk_size`

**Files:**
- Modify: `internal/config/config.go:42-47`（`ScanConfig`）
- Modify: `internal/config/config.go:113-118`（`defaultConfig` 的 `Scan` 段）
- Modify: `configs/atlas.yaml:10-14`（`scan:` 段）

**Interfaces:**
- 消费：无
- 产出：`cfg.Scan.PortChunkSize int`（后续 `main.go` 与 `task.New` 使用）

- [ ] **Step 1: 在 `ScanConfig` 增加字段**
```go
type ScanConfig struct {
	DefaultMode      string `yaml:"default_mode"`
	DefaultPortRange string `yaml:"default_port_range"`
	MaxConcurrency   int    `yaml:"max_concurrency"`
	PerTargetRPS     int    `yaml:"per_target_rps"`
	PortChunkSize    int    `yaml:"port_chunk_size"` // 单 IP 端口切块大小，默认 1000
}
```

- [ ] **Step 2: 在 `defaultConfig` 的 `Scan` 段加默认 1000**
```go
		Scan: ScanConfig{
			DefaultMode:      "connect",
			DefaultPortRange: "top1000",
			MaxConcurrency:   500,
			PerTargetRPS:     10,
			PortChunkSize:    1000,
		},
```

- [ ] **Step 3: 在 `configs/atlas.yaml` 的 `scan:` 段加一行**
```yaml
scan:
  default_mode: "connect"
  default_port_range: "top1000"
  max_concurrency: 500
  per_target_rps: 10
  port_chunk_size: 1000
```

- [ ] **Step 4: 编译验证**
Run: `cd d:\myself\scan\atlas && go build ./internal/config/`
Expected: 编译通过，无错误

- [ ] **Step 5: Commit**
```bash
git add internal/config/config.go configs/atlas.yaml
git commit -m "feat(config): add port_chunk_size for port-block task granularity"
```

---

### Task 2: 数据库迁移 `000004`

**Files:**
- Create: `migrations/000004_port_chunk.up.sql`
- Create: `migrations/000004_port_chunk.down.sql`

**Interfaces:**
- 消费：无
- 产出：`task_items.ports` 列 + 唯一约束 `task_items_task_id_target_ports_key`（后续 store 代码依赖）

- [ ] **Step 1: 写 up 迁移**
```sql
-- 任务子项支持端口块粒度（进度可见性细化到端口块）
ALTER TABLE task_items ADD COLUMN IF NOT EXISTS ports TEXT NOT NULL DEFAULT '';
ALTER TABLE task_items DROP CONSTRAINT IF EXISTS task_items_task_id_target_key;
ALTER TABLE task_items
  ADD CONSTRAINT task_items_task_id_target_ports_key UNIQUE (task_id, target, ports);
```

- [ ] **Step 2: 写 down 迁移**
```sql
ALTER TABLE task_items DROP CONSTRAINT IF EXISTS task_items_task_id_target_ports_key;
ALTER TABLE task_items DROP COLUMN IF EXISTS ports;
```

- [ ] **Step 3: Commit**
```bash
git add migrations/000004_port_chunk.up.sql migrations/000004_port_chunk.down.sql
git commit -m "feat(db): add ports column and (task_id,target,ports) unique on task_items"
```

---

### Task 3: 模型 `TaskItem.Ports`

**Files:**
- Modify: `internal/model/model.go:83-89`（`TaskItem`）

**Interfaces:**
- 消费：无
- 产出：`model.TaskItem.Ports string`（store / task 使用）

- [ ] **Step 1: 给 `TaskItem` 加 `Ports` 字段**
```go
// TaskItem 任务子项（断点续扫单元，粒度可细化到端口块）
type TaskItem struct {
	TaskID string         `json:"task_id"`
	Target string         `json:"target"`
	Ports  string         `json:"ports"`   // 端口块规格，如 "1-1000"；域名/空块为 ""
	Status int            `json:"status"` // 0 pending 1 done 2 filtered
	Result map[string]any `json:"result"`
}
```

- [ ] **Step 2: 编译验证**
Run: `cd d:\myself\scan\atlas && go build ./internal/model/`
Expected: 编译通过

- [ ] **Step 3: Commit**
```bash
git add internal/model/model.go
git commit -m "feat(model): add Ports field to TaskItem"
```

---

### Task 4: 仓储适配 `ports`

**Files:**
- Modify: `internal/store/pg.go:462-512`（`UpsertTaskItem` / `ListTaskItems` / `MarkItemDone`）

**Interfaces:**
- 消费：`model.TaskItem.Ports`（Task 3）
- 产出：`UpsertTaskItem` 写 `ports` + 新冲突键；`ListTaskItems` 读 `ports` 到 `it.Ports`；`MarkItemDone(ctx, taskID, target, ports, result)` 新签名

- [ ] **Step 1: 改写 `UpsertTaskItem`（含 `ports` 列与新冲突键）**
```go
// UpsertTaskItem 写入任务子项（冲突更新状态/结果）
func (s *Store) UpsertTaskItem(ctx context.Context, item model.TaskItem) error {
	res, _ := json.Marshal(item.Result)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO task_items (task_id, target, ports, status, result)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (task_id, target, ports) DO UPDATE SET status=EXCLUDED.status, result=EXCLUDED.result`,
		item.TaskID, item.Target, item.Ports, item.Status, res)
	return err
}
```

- [ ] **Step 2: 改写 `ListTaskItems`（`SELECT` 加 `ports`，`Scan` 顺序同步）**
```go
// ListTaskItems 列出任务子项，statusFilter 为 nil 时返回全部
func (s *Store) ListTaskItems(ctx context.Context, taskID string, statusFilter *int) ([]model.TaskItem, error) {
	var rows pgx.Rows
	var err error
	if statusFilter == nil {
		rows, err = s.pool.Query(ctx, `SELECT task_id, target, ports, status, result FROM task_items WHERE task_id=$1`, taskID)
	} else {
		rows, err = s.pool.Query(ctx, `SELECT task_id, target, ports, status, result FROM task_items WHERE task_id=$1 AND status=$2`, taskID, *statusFilter)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.TaskItem{}
	for rows.Next() {
		var it model.TaskItem
		var res []byte
		if err := rows.Scan(&it.TaskID, &it.Target, &it.Ports, &it.Status, &res); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(res, &it.Result)
		out = append(out, it)
	}
	return out, rows.Err()
}
```

- [ ] **Step 3: 改写 `MarkItemDone`（增加 `ports` 参数）**
```go
// MarkItemDone 标记子项完成并写入结果
func (s *Store) MarkItemDone(ctx context.Context, taskID, target, ports string, result map[string]any) error {
	return s.UpsertTaskItem(ctx, model.TaskItem{TaskID: taskID, Target: target, Ports: ports, Status: model.TaskItemDone, Result: result})
}
```

- [ ] **Step 4: 编译验证（此时 `task.go` 仍用旧 `MarkItemDone` 签名会编译失败，属预期——Task 6/7 会补齐；先单独 vet store 包）**
Run: `cd d:\myself\scan\atlas && go vet ./internal/store/`
Expected: store 包内无错误（`task` 包报错不影响本包 vet）

- [ ] **Step 5: Commit**
```bash
git add internal/store/pg.go
git commit -m "feat(store): adapt task_items to ports column and new unique key"
```

---

### Task 5: 队列消息 `TaskMsg.Ports`

**Files:**
- Modify: `internal/queue/nats.go:33-38`（`TaskMsg`）

**Interfaces:**
- 消费：无
- 产出：`queue.TaskMsg.Ports string`（Task 7 的 `dispatch` 与 `handler` 使用）

- [ ] **Step 1: 给 `TaskMsg` 加 `Ports` 字段**
```go
// TaskMsg 队列传递的任务消息
type TaskMsg struct {
	TaskID string `json:"task_id"`
	Target string `json:"target"`
	Ports  string `json:"ports"`
	Kind   string `json:"kind"`
}
```

- [ ] **Step 2: 编译验证**
Run: `cd d:\myself\scan\atlas && go build ./internal/queue/`
Expected: 编译通过

- [ ] **Step 3: Commit**
```bash
git add internal/queue/nats.go
git commit -m "feat(queue): add Ports to TaskMsg for port-block dispatch"
```

---

### Task 6: `Processor` 接口与扫描/漏洞引擎签名

**Files:**
- Modify: `internal/task/task.go:21-31`（`Processor` 接口 + `noopProcessor`）
- Modify: `internal/scan/scan.go:37-53`（`Process` 与 `portsFor`）
- Modify: `internal/vuln/engine.go:65`（`Engine.Process` 签名）

**Interfaces:**
- 消费：无（本任务只改签名）
- 产出：`Process(ctx, task, target, ports string)` 统一签名；`scan.Process` 优先用 `ports` 参数

- [ ] **Step 1: 改 `task.go` 的 `Processor` 接口与 `noopProcessor`**
```go
// Processor 单目标处理逻辑（Issue #4 注入真实探测实现）
type Processor interface {
	Process(ctx context.Context, task model.Task, target, ports string) (map[string]any, error)
}

// noopProcessor 占位实现，仅供调度链路自测
type noopProcessor struct{}

func (noopProcessor) Process(_ context.Context, _ model.Task, target, _ string) (map[string]any, error) {
	return map[string]any{"target": target, "skipped": "no processor"}, nil
}
```

- [ ] **Step 2: 改 `scan.go` 的 `Process`（优先用消息端口）**
```go
// Process 实现 task.Processor：根据目标类型分派
func (sc *Scanner) Process(ctx context.Context, task model.Task, target, ports string) (map[string]any, error) {
	var plist []int
	if ports != "" {
		if ps, err := ParsePortSpec(ports); err == nil && len(ps) > 0 {
			plist = ps
		}
	}
	if plist == nil {
		plist = sc.portsFor(task)
	}
	if net.ParseIP(target) != nil {
		return sc.scanHost(ctx, target, plist)
	}
	return sc.scanDomain(ctx, target, plist)
}
```

- [ ] **Step 3: 改 `vuln/engine.go` 的 `Engine.Process` 签名（忽略 `ports`，函数体不变）**
```go
func (e *Engine) Process(ctx context.Context, task model.Task, target, ports string) (map[string]any, error) {
	found := []map[string]any{}
	ports := []struct {
		port   int
		scheme string
	}{
		{80, "http"}, {8080, "http"}, {8000, "http"}, {8888, "http"},
		{443, "https"}, {8443, "https"},
	}
```
注意：原 `vuln/engine.go` 内部已声明局部变量 `ports`，与新参数 `ports` 同名会冲突。需把参数名改为 `_` 或将内部变量改名。采用参数写为 `ports string` 但函数体内不使用——但内部已有 `ports := []struct{...}`。因此把参数命名为 `_` 最简洁：
```go
func (e *Engine) Process(ctx context.Context, task model.Task, target, _ string) (map[string]any, error) {
	found := []map[string]any{}
	ports := []struct {
		port   int
		scheme string
	}{
		{80, "http"}, {8080, "http"}, {8000, "http"}, {8888, "http"},
		{443, "https"}, {8443, "https"},
	}
```
（其余函数体保持原样不变）

- [ ] **Step 4: 编译验证（此时 `task.go` 的 `processOne` 仍用旧 `Process`/`MarkItemDone` 调用，会编译失败——属预期，Task 7 补齐）**
Run: `cd d:\myself\scan\atlas && go build ./internal/scan/ ./internal/vuln/`
Expected: scan / vuln 包编译通过

- [ ] **Step 5: Commit**
```bash
git add internal/task/task.go internal/scan/scan.go internal/vuln/engine.go
git commit -m "refactor(processor): add ports param to Process interface and impls"
```

---

### Task 7: 调度核心全链路透传 + 切块

**Files:**
- Modify: `internal/task/task.go`（多处：`Service` 结构、`New`、`Create`、`dispatch`、`processOne`、`RegisterWorker` handler、`runInProcess`）
- Test: `internal/task/task_test.go`（新建，测 `chunkSpec`）

**Interfaces:**
- 消费：`model.TaskItem.Ports`（Task 3）、`queue.TaskMsg.Ports`（Task 5）、`store.MarkItemDone(ctx, taskID, target, ports, result)`（Task 4）、`Processor.Process(ctx, task, target, ports)`（Task 6）、`scan.ParsePortSpec`（已存在）
- 产出：`task.New` 新签名（main.go 调用）；`chunkSpec` 与 `portsForScope` 辅助函数；`Create` 按块建 item 且 `Progress.total` = 块数

- [ ] **Step 1: `Service` 结构加字段 + `New` 新签名**
```go
// Service 任务调度：创建、编排、断点续扫、Worker 注册
type Service struct {
	store         *store.Store
	queue         *queue.Queue
	audit         *audit.Auditor
	bl            *blacklist.Service
	rate          *ratelimit.Limiter
	scanProc      Processor
	vulnProc      Processor
	concurrency   int
	defaultPorts  []int
	portChunkSize int
}

// New 构造任务服务
func New(s *store.Store, q *queue.Queue, a *audit.Auditor, bl *blacklist.Service, r *ratelimit.Limiter, concurrency int, defaultPorts []int, portChunkSize int) *Service {
	if portChunkSize <= 0 {
		portChunkSize = 1000
	}
	return &Service{store: s, queue: q, audit: a, bl: bl, rate: r, scanProc: noopProcessor{}, vulnProc: noopProcessor{}, concurrency: concurrency, defaultPorts: defaultPorts, portChunkSize: portChunkSize}
}
```

- [ ] **Step 2: 增加 `portsForScope` 与 `chunkSpec` 辅助函数**
```go
// portsForScope 解析某任务端口列表：优先用 scope.ports，否则回退默认端口
func (svc *Service) portsForScope(sc map[string]any) []int {
	if v, ok := sc["ports"].(string); ok {
		if ps, err := scan.ParsePortSpec(v); err == nil && len(ps) > 0 {
			return ps
		}
	}
	return svc.defaultPorts
}

// chunkSpec 将端口切片按 size 切块，返回可被 ParsePortSpec 精确还原的规格字符串：
// 连续递增块用 "lo-hi"，非连续块用逗号拼接。
func chunkSpec(ports []int, size int) []string {
	if size <= 0 {
		size = 1
	}
	out := make([]string, 0, (len(ports)+size-1)/size)
	for i := 0; i < len(ports); i += size {
		end := i + size
		if end > len(ports) {
			end = len(ports)
		}
		chunk := ports[i:end]
		if isContiguous(chunk) {
			out = append(out, fmt.Sprintf("%d-%d", chunk[0], chunk[len(chunk)-1]))
		} else {
			parts := make([]string, len(chunk))
			for j, p := range chunk {
				parts[j] = strconv.Itoa(p)
			}
			out = append(out, strings.Join(parts, ","))
		}
	}
	return out
}

// isContiguous 判断切片是否严格递增 1
func isContiguous(chunk []int) bool {
	for i := 1; i < len(chunk); i++ {
		if chunk[i] != chunk[i-1]+1 {
			return false
		}
	}
	return true
}
```

- [ ] **Step 3: 改写 `Create` 的建 item 逻辑（含切块与 `Progress.total`）**
将现有 `Create` 中：
```go
	id := newID()
	task := model.Task{
		ID:        id,
		Kind:      kind,
		Scope:     sc,
		Schedule:  schedule,
		RateLimit: rateLimit,
		Status:    model.TaskRunning,
		Progress:  map[string]int{"total": len(targets), "done": 0},
	}
	if err := svc.store.CreateTask(ctx, task); err != nil {
		return "", err
	}
	for _, t := range targets {
		st := model.TaskItemPending
		if hit, _ := svc.bl.Match(ctx, t); hit {
			st = model.TaskItemFiltered
		}
		if err := svc.store.UpsertTaskItem(ctx, model.TaskItem{TaskID: id, Target: t, Status: st}); err != nil {
			return "", err
		}
	}
```
替换为：
```go
	id := newID()
	type itemSpec struct {
		target   string
		ports    string
		filtered bool
	}
	specs := make([]itemSpec, 0, len(targets))
	for _, t := range targets {
		filtered := false
		if hit, _ := svc.bl.Match(ctx, t); hit {
			filtered = true
		}
		var chunks []string
		if net.ParseIP(t) != nil {
			if plist := svc.portsForScope(sc); len(plist) > 0 {
				chunks = chunkSpec(plist, svc.portChunkSize)
			}
		}
		if len(chunks) == 0 {
			specs = append(specs, itemSpec{t, "", filtered})
		} else {
			for _, c := range chunks {
				specs = append(specs, itemSpec{t, c, filtered})
			}
		}
	}
	task := model.Task{
		ID:        id,
		Kind:      kind,
		Scope:     sc,
		Schedule:  schedule,
		RateLimit: rateLimit,
		Status:    model.TaskRunning,
		Progress:  map[string]int{"total": len(specs), "done": 0},
	}
	if err := svc.store.CreateTask(ctx, task); err != nil {
		return "", err
	}
	for _, sp := range specs {
		st := model.TaskItemPending
		if sp.filtered {
			st = model.TaskItemFiltered
		}
		if err := svc.store.UpsertTaskItem(ctx, model.TaskItem{TaskID: id, Target: sp.target, Ports: sp.ports, Status: st}); err != nil {
			return "", err
		}
	}
```
并在 `task.go` 顶部 `import` 中增加 `"net"`、`"strconv"`、`"strings"` 与 `"atlas/internal/scan"`。

- [ ] **Step 4: 改写 `dispatch` 发布消息携带 `Ports`**
将：
```go
		for _, it := range items {
			msg := TaskMsg{TaskID: taskID, Target: it.Target, Kind: subject}
```
改为：
```go
		for _, it := range items {
			msg := TaskMsg{TaskID: taskID, Target: it.Target, Ports: it.Ports, Kind: subject}
```

- [ ] **Step 5: 改写 `processOne` 签名与内部调用**
将：
```go
func (svc *Service) processOne(ctx context.Context, task model.Task, target string) {
```
改为：
```go
func (svc *Service) processOne(ctx context.Context, task model.Task, target, ports string) {
```
并将其中：
```go
	res, err := proc.Process(ctx, task, target)
```
改为：
```go
	res, err := proc.Process(ctx, task, target, ports)
```
将：
```go
	_ = svc.store.MarkItemDone(ctx, task.ID, target, res)
```
改为：
```go
	_ = svc.store.MarkItemDone(ctx, task.ID, target, ports, res)
```

- [ ] **Step 6: 改写 `RegisterWorker` handler 调用**
将：
```go
			svc.processOne(context.Background(), task, msg.Target)
```
改为：
```go
			svc.processOne(context.Background(), task, msg.Target, msg.Ports)
```

- [ ] **Step 7: 改写 `runInProcess` 调用**
将：
```go
					svc.processOne(ctx, task, it.Target)
```
改为：
```go
					svc.processOne(ctx, task, it.Target, it.Ports)
```

- [ ] **Step 8: 写失败单测 `internal/task/task_test.go`**
```go
package task

import (
	"testing"

	"atlas/internal/scan"
)

func TestChunkSpecContiguous(t *testing.T) {
	ports := make([]int, 65535)
	for i := range ports {
		ports[i] = i + 1
	}
	chunks := chunkSpec(ports, 1000)
	if len(chunks) != 66 {
		t.Fatalf("expected 66 chunks, got %d", len(chunks))
	}
	if chunks[0] != "1-1000" {
		t.Fatalf("first chunk = %q, want 1-1000", chunks[0])
	}
	if chunks[65] != "65001-65535" {
		t.Fatalf("last chunk = %q, want 65001-65535", chunks[65])
	}
	// 还原校验：ParsePortSpec 必须精确重建原集合
	got, err := scan.ParsePortSpec(chunks[0])
	if err != nil || len(got) != 1000 || got[0] != 1 || got[999] != 1000 {
		t.Fatalf("round-trip failed: %v len=%d", err, len(got))
	}
}

func TestChunkSpecScattered(t *testing.T) {
	// 模拟 TopPorts 这种非连续集合
	ports := []int{21, 22, 23, 53, 80, 110, 443}
	chunks := chunkSpec(ports, 1000) // 单块（超过长度）
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	got, err := scan.ParsePortSpec(chunks[0])
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(got) != len(ports) {
		t.Fatalf("round-trip mismatch: got %d ports, want %d", len(got), len(ports))
	}
	for i, p := range got {
		if p != ports[i] {
			t.Fatalf("round-trip order mismatch at %d: %d != %d", i, p, ports[i])
		}
	}
}

func TestChunkSpecSizeOne(t *testing.T) {
	ports := []int{10, 20, 30}
	chunks := chunkSpec(ports, 1)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0] != "10-10" || chunks[2] != "30-30" {
		t.Fatalf("unexpected chunks: %v", chunks)
	}
}
```

- [ ] **Step 9: 运行单测确认通过**
Run: `cd d:\myself\scan\atlas && go test ./internal/task/ -run TestChunkSpec -v`
Expected: 三个测试全部 PASS

- [ ] **Step 10: 全量编译验证**
Run: `cd d:\myself\scan\atlas && go build ./...`
Expected: 全部编译通过（此前 Task 4/6 的预期失败已闭合）

- [ ] **Step 11: Commit**
```bash
git add internal/task/task.go internal/task/task_test.go
git commit -m "feat(task): chunk ports into TaskItems and thread ports through pipeline"
```

---

### Task 8: `main.go` 装配

**Files:**
- Modify: `cmd/atlas/main.go:102`（`task.New` 调用）

**Interfaces:**
- 消费：`task.New` 新签名（Task 7）、`cfg.Scan.PortChunkSize`（Task 1）、`defaultPorts`（已在 `main.go` 计算）
- 产出：无

- [ ] **Step 1: 改 `task.New` 调用，补 `defaultPorts` 与 `cfg.Scan.PortChunkSize`**
将：
```go
	taskSvc := task.New(st, q, auditor, bl, limiter, cfg.Scan.MaxConcurrency)
```
改为：
```go
	taskSvc := task.New(st, q, auditor, bl, limiter, cfg.Scan.MaxConcurrency, defaultPorts, cfg.Scan.PortChunkSize)
```

- [ ] **Step 2: 全量编译验证**
Run: `cd d:\myself\scan\atlas && go build ./...`
Expected: 编译通过

- [ ] **Step 3: Commit**
```bash
git add cmd/atlas/main.go
git commit -m "feat(main): wire defaultPorts and port_chunk_size into task service"
```

---

### Task 9: 集成验证（本机 docker）

**Files:** 无代码改动

**Interfaces:**
- 消费：全部前述改动

- [ ] **Step 1: 重建镜像**
Run: `cd d:\myself\scan\atlas && docker-compose up --build -d`
Expected: 容器用新镜像重启

- [ ] **Step 2: 创建全端口任务并轮询进度**
Run（PowerShell）:
```powershell
cd d:\myself\scan\atlas
$session=New-Object Microsoft.PowerShell.Commands.WebRequestSession
Invoke-RestMethod -Uri http://localhost:8080/api/login -Method Post -ContentType "application/json" -Body '{"password":"admin"}' -WebSession $session | Out-Null
$body='{"kind":"scan","scope":{"targets":["127.0.0.1"],"ports":"1-65535"}}'
$r=Invoke-RestMethod -Uri http://localhost:8080/api/tasks -Method Post -ContentType "application/json" -Body $body -WebSession $session
$id=$r.id
Write-Host "TASK_ID=$id"
1..12 | ForEach-Object {
  Start-Sleep -Seconds 20
  $s=Invoke-RestMethod -Uri "http://localhost:8080/api/tasks/$id" -WebSession $session
  Write-Host ("poll {0}: status={1} progress={2}/{3}" -f $_, $s.task.status, $s.task.progress.done, $s.task.progress.total)
}
```
Expected: `progress.done` 从 0 逐步上涨（最终应到 66，不再长时间卡 0），`progress.total=66`。

- [ ] **Step 3: 核对 `task_items` 行数与 `ports` 值**
Run（PowerShell）:
```powershell
docker-compose exec -T postgres psql -U postgres -d atlas -tAc "SELECT count(*), string_agg(ports, '|' ORDER BY ports) FROM task_items WHERE task_id='<上一步的TASK_ID>'"
```
Expected: `count=66`，`ports` 含 `1-1000` … `65001-65535`。

- [ ] **Step 4: 回归小范围与域名**
Run: 创建 `{"kind":"scan","scope":{"targets":["127.0.0.1"],"ports":"1-100"}}` 与 `{"kind":"scan","scope":{"targets":["example.com"]}}`，各轮询确认 `progress.total=1` 且最终 `status=2`（Done）。
Expected: 两个任务均正常 `0/1 → 1/1` 完成，无报错、无 panic（atlas2 日志无 `recovered panic`）。

- [ ] **Step 5: Commit（仅文档/验证结论，可选）**
```bash
git add -A
git commit -m "test: verify port-chunk progress smoothness on 1-65535 scan" || echo "nothing to commit"
```

---

## Self-Review

**1. Spec coverage:**
- config `port_chunk_size` 默认 1000 → Task 1 ✅
- migration 加 `ports` 列 + 新唯一约束 → Task 2 ✅
- `TaskItem.Ports` → Task 3 ✅
- store 三个函数适配 → Task 4 ✅
- `TaskMsg.Ports` → Task 5 ✅
- `Processor.Process` 新签名 + scan/vuln/noop → Task 6 ✅
- `Create` 切块 + 全链路透传 `ports` → Task 7 ✅
- `main.go` 装配 → Task 8 ✅
- 验证（平滑 / 回归）→ Task 9 ✅

**2. Placeholder scan:** 无 TBD/TODO；每个代码步骤均含完整实现或测试代码。✅

**3. Type consistency:**
- `MarkItemDone(ctx, taskID, target, ports, result)` 在 Task 4 定义、Task 7 调用一致 ✅
- `TaskMsg.Ports` 在 Task 5 定义、Task 7 读写一致 ✅
- `Processor.Process(ctx, task, target, ports)` 接口（Task 6）与 `scan`/`vuln`/`noop` 实现（Task 6）及 `processOne` 调用（Task 7）一致 ✅
- `chunkSpec(ports []int, size int) []string` 在 Task 7 定义、Task 7 测试使用一致 ✅
- `task.New(..., defaultPorts []int, portChunkSize int)` 在 Task 7 定义、Task 8 调用一致 ✅

**发现并修正的问题：** Task 6 中 `vuln/engine.go` 原函数体已声明局部变量 `ports`，与新参数同名。已在计划中明确采用参数名 `_` 规避冲突，函数体其余不变。
