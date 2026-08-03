# LOG_LEVEL 环境变量控制与 Docker 集成 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `cmd/atlas/main.go` 中从环境变量 `LOG_LEVEL` 动态读取日志等级，并在 `docker-compose.yml` 中注入 `LOG_LEVEL: "debug"` 环境变量。

## Global Constraints

- 不改变既有测试行为。
- 全量 `go test ./...` PASS。

---

### Task 1: `cmd/atlas/main.go` 支持读取 `LOG_LEVEL` 环境变量

**Files:**
- Modify: `cmd/atlas/main.go`

- [ ] **Step 1: 读取 `LOG_LEVEL` 并设置 logger 等级**

在 `cmd/atlas/main.go` 中：
```go
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		logger.SetLevel(lvl)
	}
```

- [ ] **Step 2: 运行全量测试验证**

Run: `go test ./...`
Expected: PASS

---

### Task 2: 在 `docker-compose.yml` 中增加 `LOG_LEVEL` 环境变量

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: 修改 docker-compose.yml**

为 `atlas` 和 `atlas2` 服务增加 `LOG_LEVEL: "debug"` 环境变量：
```yaml
    environment:
      LOG_LEVEL: "debug"
      ATLAS_PG_DSN: "postgres://postgres:postgres@postgres:5432/atlas?sslmode=disable"
      ATLAS_NATS_URL: "nats://nats:4222"
      ATLAS_ES_ADDR: "http://elasticsearch:9200"
```
