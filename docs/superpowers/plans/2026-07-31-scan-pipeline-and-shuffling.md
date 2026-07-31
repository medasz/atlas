# 扫描引擎三级流水线解耦与 IP 打散 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 重构 Atlas 资产扫描引擎，将其解耦为“纯端口发现”、“异步 HTTP/指纹探测”、“批量 Bulk 落库”三级流水线，并引入 IP 随机打散，提升十倍以上扫描吞吐量。

**Architecture:** 端口发包不阻塞等待 HTTP 探测与写库。发现端口通过 Channel 扔给后台 Enricher 协程池做 HTTP/指纹分析，最后通过 Batch Writer 批量 Bulk 落库。

**Tech Stack:** Go 1.22+ (Channels, Goroutines, sync.Mutex, Fisher-Yates Shuffling)

## Global Constraints

- 不降低探测精度（HTTP 探测、证书与指纹仍精准抓取）。
- 保证测试覆盖率，全量 `go test ./...` PASS。

---

### Task 1: 实现目标 IP 随机打散 (IP Shuffling)

**Files:**
- Modify: `internal/scope/scope.go`
- Test: `internal/scope/scope_test.go`

**Interfaces:**
- Produces: `ShuffleIPs(ips []string) []string`

- [ ] **Step 1: 编写 ShuffleIPs 单元测试**

在 `internal/scope/scope_test.go` 中添加测试：
```go
func TestShuffleIPs(t *testing.T) {
	ips := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3", "4.4.4.4", "5.5.5.5"}
	shuffled := ShuffleIPs(ips)
	if len(shuffled) != len(ips) {
		t.Fatalf("expected len=%d, got %d", len(ips), len(shuffled))
	}
	// 验证元素全集一致
	m := map[string]bool{}
	for _, v := range shuffled {
		m[v] = true
	}
	for _, v := range ips {
		if !m[v] {
			t.Errorf("missing ip %s in shuffled output", v)
		}
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/scope/... -v`
Expected: FAIL with "undefined: ShuffleIPs"

- [ ] **Step 3: 实现 ShuffleIPs 随机打散算法**

在 `internal/scope/scope.go` 中实现 Fisher-Yates 打散：
```go
// ShuffleIPs 对 IP 列表进行随机打散
func ShuffleIPs(ips []string) []string {
	out := make([]string, len(ips))
	copy(out, ips)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := len(out) - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return out
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/scope/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/scope/
git commit -m "feat(scope): add ShuffleIPs randomized shuffling"
```

---

### Task 2: 实现异步 Enricher 池与 Batch Bulk Writer 机制

**Files:**
- Create: `internal/scan/pipeline.go`
- Test: `internal/scan/pipeline_test.go`

**Interfaces:**
- Produces: `openPortEvent`, `startPipeline`, `enrichWorker`, `batchWriter`

- [ ] **Step 1: 编写 Pipeline 管道单元测试**

创建 `internal/scan/pipeline_test.go`：
```go
package scan

import (
	"testing"
	"time"
)

func TestPipelineChannel(t *testing.T) {
	events := make(chan openPortEvent, 10)
	events <- openPortEvent{IP: "127.0.0.1", Port: 80, Banner: "HTTP"}
	close(events)

	e := <-events
	if e.IP != "127.0.0.1" || e.Port != 80 {
		t.Errorf("expected 127.0.0.1:80, got %s:%d", e.IP, e.Port)
	}
}
```

- [ ] **Step 2: 实现 internal/scan/pipeline.go 异步消费逻辑**

创建 `internal/scan/pipeline.go`：
```go
package scan

import (
	"context"
	"log"
	"time"

	"atlas/internal/model"
)

type openPortEvent struct {
	IP     string
	Port   int
	Banner string
	IsV6   bool
}

// startPipeline 启动异步 HTTP 探测 Worker 池与 Batch Writer
func (sc *Scanner) startPipeline(ctx context.Context) {
	sc.enrichChan = make(chan openPortEvent, 10000)
	sc.writerChan = make(chan model.Asset, 10000)

	// 启动 50 个 HTTP/指纹探测 Worker
	for i := 0; i < 50; i++ {
		go sc.enrichWorker(ctx)
	}

	// 启动 1 个批量写入器
	go sc.batchWriter(ctx)
}

func (sc *Scanner) enrichWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-sc.enrichChan:
			if !ok {
				return
			}
			portAsset := model.Asset{
				IP:       evt.IP,
				Port:     evt.Port,
				Proto:    "tcp",
				State:    "open",
				Service:  guessService(evt.Port, evt.Banner),
				Banner:   evt.Banner,
				Host:     evt.IP,
				IsIPv6:   evt.IsV6,
				LastSeen: time.Now(),
			}
			sc.httpEnrich(evt.IP, evt.Port, evt.Banner, &portAsset)
			select {
			case sc.writerChan <- portAsset:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (sc *Scanner) batchWriter(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	batch := make([]model.Asset, 0, 50)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		for _, a := range batch {
			sc.upsert(ctx, a)
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case a, ok := <-sc.writerChan:
			if !ok {
				flush()
				return
			}
			batch = append(batch, a)
			if len(batch) >= 50 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
```

- [ ] **Step 3: 运行测试验证通过**

Run: `go test ./internal/scan/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/scan/
git commit -m "feat(scan): add pipeline async enricher and batch writer"
```

---

### Task 3: 改造 Scanner 提升并发与集成 Pipeline

**Files:**
- Modify: `internal/scan/scan.go`

- [ ] **Step 1: 在 scan.go 中增大默认并发并集成异步事件投递**

修改 `internal/scan/scan.go`：
1. 将 `connSem` 从 50 提升至 **200**；
2. 在 `New` 中初始化 pipeline 管道与启动协程；
3. 在 `persistResult` 中不再同步调用 `httpEnrich` 和 `sc.upsert`，改为将 `openPortEvent` 投递至 `enrichChan`。

- [ ] **Step 2: 全量测试验证**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/scan/
git commit -m "refactor(scan): decouple port scan from HTTP enrich and increase concurrency"
```
