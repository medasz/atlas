# 配置单 Key-Value 扁平存储 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 PostgreSQL 中 `config` 表由整块 JSONB 存储重构为下划线前缀单项 KEY/VALUE（TEXT 类型）存储，保持 HTTP API 接口 JSON 分组格式不变。

**Architecture:** 数据库只存扁平的 `(key TEXT, value TEXT)` 记录。`internal/config` 包负责 Struct 结构与扁平下划线 KEY 字典的双向转换解析。

**Tech Stack:** Go 1.22+, PostgreSQL (pgx/v5)

## Global Constraints

- 不改变前端 `GET/PUT /api/v1/config` 的 JSON 分组数据结构。
- 数据库 `config.value` 必须为 `TEXT` 类型。
- 配置键名必须带有模块前缀下划线（如 `scan_default_mode`）。

---

### Task 1: 更新数据库 Migration 脚本中 config 表定义

**Files:**
- Modify: `migrations/000001_schema.up.sql`

**Interfaces:**
- Produces: 具有 `(key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TIMESTAMPTZ)` 结构的 `config` 表。

- [ ] **Step 1: 修改 000001_schema.up.sql 中 config 表定义**

```sql
-- 9. 配置
CREATE TABLE IF NOT EXISTS config (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 2: Commit**

```bash
git add migrations/000001_schema.up.sql
git commit -m "schema: update config table value type to TEXT"
```

---

### Task 2: 改造 internal/config/config.go 映射与读写逻辑

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: PostgreSQL DB pool
- Produces: `LoadFromDB`, `EnsureSeeded`, `UpsertSection`

- [ ] **Step 1: 编写单元测试验证配置扁平化与加载**

创建 `internal/config/config_test.go`：
```go
package config

import (
	"testing"
)

func TestConfigKVMapping(t *testing.T) {
	cfg := defaultConfig()
	kv := configToKV(cfg)

	if kv["scan_default_mode"] != "connect" {
		t.Errorf("expected scan_default_mode=connect, got %s", kv["scan_default_mode"])
	}
	if kv["scan_max_concurrency"] != "500" {
		t.Errorf("expected scan_max_concurrency=500, got %s", kv["scan_max_concurrency"])
	}
	if kv["auth_enabled"] != "true" {
		t.Errorf("expected auth_enabled=true, got %s", kv["auth_enabled"])
	}

	// 验证反解析
	newCfg := defaultConfig()
	kv["scan_default_mode"] = "syn"
	kv["scan_max_concurrency"] = "1000"
	applyKVToConfig(newCfg, kv)

	if newCfg.Scan.DefaultMode != "syn" {
		t.Errorf("expected syn, got %s", newCfg.Scan.DefaultMode)
	}
	if newCfg.Scan.MaxConcurrency != 1000 {
		t.Errorf("expected 1000, got %d", newCfg.Scan.MaxConcurrency)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/config/... -v`
Expected: FAIL with "undefined: configToKV"

- [ ] **Step 3: 实现 internal/config/config.go 转换与读写逻辑**

在 `internal/config/config.go` 中实现 `configToKV`、`applyKVToConfig`、`UpsertSection` 和 `LoadFromDB`：

```go
// configToKV 将 Config 结构体转为下划线 KEY 的字符串 Map
func configToKV(cfg *Config) map[string]string {
	return map[string]string{
		"scan_default_mode":          cfg.Scan.DefaultMode,
		"scan_default_port_range":   cfg.Scan.DefaultPortRange,
		"scan_max_concurrency":     fmt.Sprintf("%d", cfg.Scan.MaxConcurrency),
		"scan_per_target_rps":        fmt.Sprintf("%d", cfg.Scan.PerTargetRPS),
		"scan_port_chunk_size":       fmt.Sprintf("%d", cfg.Scan.PortChunkSize),
		"scan_raw_capture_window_sec": fmt.Sprintf("%d", cfg.Scan.RawCaptureWindowSec),
		"scan_raw_retries":           fmt.Sprintf("%d", cfg.Scan.RawRetries),
		"scan_record_filtered_ports": fmt.Sprintf("%t", cfg.Scan.RecordFilteredPorts),
		"scan_record_closed_ports":   fmt.Sprintf("%t", cfg.Scan.RecordClosedPorts),
		"scan_install_rst_drop":      fmt.Sprintf("%t", cfg.Scan.InstallRstDrop),
		"scan_raw_iface":             cfg.Scan.RawIface,
		"auth_enabled":               fmt.Sprintf("%t", cfg.Auth.Enabled),
		"auth_password":              cfg.Auth.Password,
		"auth_secret":                cfg.Auth.Secret,
		"audit_enabled":              fmt.Sprintf("%t", cfg.Audit.Enabled),
		"http_addr":                  cfg.HTTP.Addr,
	}
}

// applyKVToConfig 将 KV 字典解析填入 Config
func applyKVToConfig(cfg *Config, kv map[string]string) {
	for k, v := range kv {
		switch k {
		case "scan_default_mode":
			cfg.Scan.DefaultMode = v
		case "scan_default_port_range":
			cfg.Scan.DefaultPortRange = v
		case "scan_max_concurrency":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Scan.MaxConcurrency = n
			}
		case "scan_per_target_rps":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Scan.PerTargetRPS = n
			}
		case "scan_port_chunk_size":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Scan.PortChunkSize = n
			}
		case "scan_raw_capture_window_sec":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Scan.RawCaptureWindowSec = n
			}
		case "scan_raw_retries":
			if n, err := strconv.Atoi(v); err == nil {
				cfg.Scan.RawRetries = n
			}
		case "scan_record_filtered_ports":
			if b, err := strconv.ParseBool(v); err == nil {
				cfg.Scan.RecordFilteredPorts = b
			}
		case "scan_record_closed_ports":
			if b, err := strconv.ParseBool(v); err == nil {
				cfg.Scan.RecordClosedPorts = b
			}
		case "scan_install_rst_drop":
			if b, err := strconv.ParseBool(v); err == nil {
				cfg.Scan.InstallRstDrop = b
			}
		case "scan_raw_iface":
			cfg.Scan.RawIface = v
		case "auth_enabled":
			if b, err := strconv.ParseBool(v); err == nil {
				cfg.Auth.Enabled = b
			}
		case "auth_password":
			cfg.Auth.Password = v
		case "auth_secret":
			cfg.Auth.Secret = v
		case "audit_enabled":
			if b, err := strconv.ParseBool(v); err == nil {
				cfg.Audit.Enabled = b
			}
		case "http_addr":
			cfg.HTTP.Addr = v
		}
	}
}
```

改造 `UpsertSection` 与 `LoadFromDB` / `EnsureSeeded`，逐项独立操作下划线 KEY。

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/config/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): implement single KV storage mapping with underscore prefix"
```

---

### Task 3: 适配 internal/store/pg.go 与 HTTP server API

**Files:**
- Modify: `internal/store/pg.go`
- Modify: `internal/server/config.go`

- [ ] **Step 1: 适配 Store.UpsertConfigSection 支持单个/多项 KV 写入**

修改 `internal/store/pg.go` 中的 `UpsertConfigSection` 方法，使其将解出的 KV 写入 `config` 表。

- [ ] **Step 2: 全量测试验证**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/store/pg.go internal/server/config.go
git commit -m "feat(server): update config API to support KV storage"
```
