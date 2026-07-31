# 配置单 Key-Value 扁平存储设计方案

- 日期：2026-07-31
- 状态：已评审通过
- 目标：将 PostgreSQL 中 `config` 表由整块 JSONB 存储改为基于下划线前缀单项 KEY/VALUE（TEXT 类型）存储，保持 HTTP API 前端 JSON 分组结构不变。

## 1. 背景与目标

现状：`config` 表结构为 `(key TEXT, value JSONB, updated_at TIMESTAMPTZ)`，存储四大配置段（`scan` / `auth` / `audit` / `http`），每个 `value` 为一整块 JSON 字符串。
问题：不便于针对单配置项做数据库粒度的查询、对比与精细化更新。

目标：
1. `config` 表 `value` 字段从 `JSONB` 修改为 `TEXT`。
2. 配置 Key 采用 `[模块前缀]_[字段名]` 下划线单项存储（如 `scan_default_mode`, `auth_enabled`）。
3. 前端 HTTP API（`GET/PUT /api/v1/config`）依然保持现有的 JSON 分组格式（如 `{"scan": {...}, "auth": {...}}`），由后端解耦转换。

## 2. 数据库 Schema 规范

在 `migrations/000001_schema.up.sql` 中更新 `config` 表：

```sql
CREATE TABLE IF NOT EXISTS config (
    key        TEXT PRIMARY KEY,  -- 如 'scan_default_mode', 'auth_enabled', 'http_addr'
    value      TEXT NOT NULL,     -- 标量文本表示，如 'connect', '500', 'true'
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## 3. 下划线键名与结构体映射字典

所有配置属性展开为下划线 KEY 存放于数据库中：

| 数据库 Key | 字段数据类型 | 默认初始值 | 对应的 Config 结构体属性 |
| :--- | :--- | :--- | :--- |
| `scan_default_mode` | string | `"connect"` | `Config.Scan.DefaultMode` |
| `scan_default_port_range` | string | `"top1000"` | `Config.Scan.DefaultPortRange` |
| `scan_max_concurrency` | int | `"500"` | `Config.Scan.MaxConcurrency` |
| `scan_per_target_rps` | int | `"10"` | `Config.Scan.PerTargetRPS` |
| `scan_port_chunk_size` | int | `"1000"` | `Config.Scan.PortChunkSize` |
| `scan_raw_capture_window_sec` | int | `"3"` | `Config.Scan.RawCaptureWindowSec` |
| `scan_raw_retries` | int | `"1"` | `Config.Scan.RawRetries` |
| `scan_record_filtered_ports` | bool | `"true"` | `Config.Scan.RecordFilteredPorts` |
| `scan_record_closed_ports` | bool | `"false"` | `Config.Scan.RecordClosedPorts` |
| `scan_install_rst_drop` | bool | `"true"` | `Config.Scan.InstallRstDrop` |
| `scan_raw_iface` | string | `""` | `Config.Scan.RawIface` |
| `auth_enabled` | bool | `"true"` | `Config.Auth.Enabled` |
| `auth_password` | string | `"admin"` | `Config.Auth.Password` |
| `auth_secret` | string | `"atlas-dev-secret-change-me"` | `Config.Auth.Secret` |
| `audit_enabled` | bool | `"true"` | `Config.Audit.Enabled` |
| `http_addr` | string | `":8080"` | `Config.HTTP.Addr` |

## 4. 转换与加载实现机制 (`internal/config/config.go`)

1. **配置播种与更新 (`EnsureSeeded` / `UpsertSection`)**：
   - 遍历各配置模块 Struct 的字段，转为对应的下划线 KEY。
   - 使用 SQL 逐项更新：
     ```sql
     INSERT INTO config (key, value, updated_at) VALUES ($1, $2, now())
     ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()
     ```

2. **读取与加载 (`LoadFromDB`)**：
   - 全量从数据库执行 `SELECT key, value FROM config` 获取全量字典。
   - 按 KEY 的前缀分类，通过类型转换（`strconv.Atoi`, `strconv.ParseBool`）回填入 `Config` 结构体中。

3. **HTTP 服务与逻辑控制 (`internal/server/config.go`)**：
   - API 保持不变：`GET /api/v1/config` 与 `PUT /api/v1/config/:section`。
   - 更新某段配置（如 `scan`）时，只更新数据库中对应 `scan_` 前缀的键值。
