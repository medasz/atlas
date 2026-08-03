# 全过程终端日志增强 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于 Go 原生 `log/slog` 构建终端高亮结构化日志包 `internal/logger`，并在服务启动、任务调度、端口扫描发包、异步 HTTP/指纹探测及 Bulk 写库的全过程进行日志埋点，提升调试与排错效率。

**Tech Stack:** Go 1.22+ (`log/slog`, ANSI Color Formatting, Atomic LogLevel)

## Global Constraints

- 日志输出高可用，零 Panic 风险。
- 日志级别可通过 `LOG_LEVEL` 环境变量或代码灵活调整。
- 全量 `go test ./...` PASS。

---

### Task 1: 构建 `internal/logger` 高亮结构化日志包

**Files:**
- Create: `internal/logger/logger.go`
- Test: `internal/logger/logger_test.go`

**Interfaces:**
- Produces: `logger.Debug(...)`, `logger.Info(...)`, `logger.Warn(...)`, `logger.Error(...)`, `logger.SetLevel(levelStr)`

- [ ] **Step 1: 编写 logger 包的单元测试**

在 `internal/logger/logger_test.go` 中编写测试：
```go
package logger

import (
	"testing"
)

func TestLoggerOutput(t *testing.T) {
	SetLevel("debug")
	Debug("test debug msg", "key", "val")
	Info("test info msg", "target", "127.0.0.1", "port", 80)
	Warn("test warn msg", "err", "timeout")
	Error("test error msg", "code", 500)
}
```

- [ ] **Step 2: 实现 `internal/logger/logger.go`**

在 `internal/logger/logger.go` 中实现基于 `slog` 的带有 ANSI 颜色的日志：
```go
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

var defaultLevel atomic.Value

func init() {
	defaultLevel.Store(slog.LevelInfo)
	setupLogger()
}

func setupLogger() {
	handler := &ansiHandler{
		writer: os.Stdout,
	}
	slog.SetDefault(slog.New(handler))
}

func SetLevel(levelStr string) {
	switch strings.ToLower(levelStr) {
	case "debug":
		defaultLevel.Store(slog.LevelDebug)
	case "warn":
		defaultLevel.Store(slog.LevelWarn)
	case "error":
		defaultLevel.Store(slog.LevelError)
	default:
		defaultLevel.Store(slog.LevelInfo)
	}
}

type ansiHandler struct {
	writer io.Writer
}

func (h *ansiHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	minLvl, ok := defaultLevel.Load().(slog.Level)
	if !ok {
		return lvl >= slog.LevelInfo
	}
	return lvl >= minLvl
}

func (h *ansiHandler) Handle(_ context.Context, r slog.Record) error {
	var colorPrefix string
	switch r.Level {
	case slog.LevelDebug:
		colorPrefix = "\033[36m[DEBUG]\033[0m" // 灰蓝/青色
	case slog.LevelInfo:
		colorPrefix = "\033[32m[INFO ]\033[0m" // 翠绿
	case slog.LevelWarn:
		colorPrefix = "\033[33m[WARN ]\033[0m" // 暖黄
	case slog.LevelError:
		colorPrefix = "\033[31m[ERROR]\033[0m" // 红色
	default:
		colorPrefix = "[LOG]"
	}

	timeStr := r.Time.Format("2006-01-02 15:04:05.000")
	var attrs strings.Builder
	r.Attrs(func(a slog.Attr) bool {
		attrs.WriteString(fmt.Sprintf(" %s=%v", a.Key, a.Value.Any()))
		return true
	})

	_, err := fmt.Fprintf(h.writer, "%s %s %s%s\n", timeStr, colorPrefix, r.Message, attrs.String())
	return err
}

func (h *ansiHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *ansiHandler) WithGroup(name string) slog.Handler      { return h }

func Debug(msg string, args ...any) { slog.Debug(msg, args...) }
func Info(msg string, args ...any)  { slog.Info(msg, args...) }
func Warn(msg string, args ...any)  { slog.Warn(msg, args...) }
func Error(msg string, args ...any) { slog.Error(msg, args...) }
```

- [ ] **Step 3: 运行测试验证**

Run: `go test ./internal/logger/... -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/logger/
git commit -m "feat(logger): add structured ANSI colored logger package"
```

---

### Task 2: 任务调度全流程日志埋点 (`internal/task/task.go`)

**Files:**
- Modify: `internal/task/task.go`

- [ ] **Step 1: 在 Create / Run / Complete 中埋点**

在大任务创建、调度执行、完成及失败处输出日志：
- 创建任务：`logger.Info("任务创建成功", "task_id", id, "kind", kind, "targets_count", len(targets))`
- 开始执行：`logger.Info("开始执行任务", "task_id", task.ID, "kind", task.Kind)`
- 任务完成：`logger.Info("任务执行完成", "task_id", task.ID, "status", task.Status, "cost_ms", elapsed)`

- [ ] **Step 2: 运行测试验证**

Run: `go test ./internal/task/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/task/
git commit -m "feat(task): add task lifecycle execution logging"
```

---

### Task 3: 扫描发包与发现全流程日志埋点 (`internal/scan/scan.go`)

**Files:**
- Modify: `internal/scan/scan.go`

- [ ] **Step 1: 埋点端口扫描发包与发现逻辑**

- 开始扫描 IP：`logger.Debug("开始扫描主机目标", "target", target, "ports_count", len(plist), "mode", sc.liveScanCfg().DefaultMode)`
- 发现 Open 端口：`logger.Info("发现开放端口", "target", ip, "port", p, "banner", banner)`
- 模式回退：`logger.Warn("扫描模式降级", "ip", ip, "reason", "IPv6 or Raw fallback")`

- [ ] **Step 2: 运行测试验证**

Run: `go test ./internal/scan/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/scan/
git commit -m "feat(scan): add port discovery and probe logging"
```

---

### Task 4: 异步 Pipeline HTTP 探测与 Bulk 落库日志埋点 (`internal/scan/pipeline.go`)

**Files:**
- Modify: `internal/scan/pipeline.go`

- [ ] **Step 1: 埋点 HTTP 指纹探测与 Bulk 写库**

- HTTP 探测完成：`logger.Debug("HTTP指纹探测完成", "target", evt.IP, "port", evt.Port, "title", portAsset.WebTitle, "status", portAsset.StatusCode)`
- Bulk 批量落库：`logger.Info("资产批量落库完成", "count", len(batch), "cost_ms", elapsed)`

- [ ] **Step 2: 运行测试验证**

Run: `go test ./internal/scan/... -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/scan/
git commit -m "feat(scan): add pipeline HTTP probe and batch flush logging"
```

---

### Task 5: 接入服务启动全量测试

**Files:**
- Modify: `cmd/atlas/main.go`

- [ ] **Step 1: cmd/atlas/main.go 初始化日志输出**

输出启动信息：`logger.Info("Atlas 系统服务已就绪", "http_addr", cfg.HTTP.Addr, "scan_mode", cfg.Scan.DefaultMode)`

- [ ] **Step 2: 运行全量测试**

Run: `go test ./...`
Expected: PASS
