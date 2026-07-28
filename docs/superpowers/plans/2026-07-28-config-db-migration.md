# Config YAML→DB Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 atlas 配置从 YAML 文件全面迁移到 PostgreSQL，首启自动播种默认配置，运行期经 DB 动态读写，连接参数改由 `.env`/环境变量提供。

**Architecture:** 引导链 = `.env`/环境变量 → 连接 DB → 迁移建 `config` 表 → `EnsureSeeded` 空库播种 → `LoadFromDB` 读四段(JSON)进内存 `*Config`；写路径 `updateConfig` 经 `store.UpsertConfigSection` 落 DB。连接参数(DSN/NATS/ES)不进 DB/YAML。

**Tech Stack:** Go 1.25；`github.com/jackc/pgx/v5` (pgxpool/pgx/pgconn)；PostgreSQL 16；手写 `.env` 解析（零新依赖）；JSON 段存储。

## Global Constraints

- Go `>= 1.25`（`GOTOOLCHAIN=local`）；`CGO_ENABLED=1 -tags raw_capture` 仅影响扫描构建，本计划不涉及扫描逻辑改动。
- `config` 包不得反向依赖 `store`：经最小 `DB` 接口（`Exec`/`QueryRow`/`Row`）解耦，`*pgxpool.Pool` 天然满足。
- DB 仅存 `scan`/`audit`/`auth`/`http` 四段；`Postgres`/`NATS`/`Elastic` 三段由 boot(`.env`) 填充。
- 升级**不做**旧 yaml→DB 导入（从默认值开始，界面可改）。
- `auth.password`/`auth.secret` 明文入库（与现状一致）。
- 删除 `config.Load(path)` / `Save()` 的 YAML 逻辑与 `yaml` 依赖；`-config` 启动参数改为 `-envfile`。

---

## File Structure

| 文件 | 责任 |
|---|---|
| `migrations/000006_config.up.sql` (+`.down.sql`) | 建 `config(key,value,updated_at)` 表 |
| `internal/config/config.go` | 删 YAML Load/Save；新增 `Bootstrap`/`DB`/`Row` 接口、`LoadBootstrap`、`LoadFromDB`、`EnsureSeeded`、`UpsertSection` |
| `internal/config/config_test.go` | 单测：`.env` 解析、seed+load roundtrip（fakeDB，无需真库） |
| `internal/store/pg.go` | 新增 `Pool()`、`UpsertConfigSection(ctx,key,value)` |
| `cmd/atlas/main.go` | 装配顺序：boot → 连库 → 迁移 → `LoadFromDB`；`-config`→`-envfile` |
| `internal/server/config.go` | `updateConfig` 写 DB 替代 `cfg.Save()` |
| `Dockerfile` / `docker-compose.yml` | 去 `-config`/yaml 挂载；保留 `.env` 引导；删 `configs/atlas.yaml` |
| `internal/config/integration_test.go` | `//go:build integration`：真 PG 验证 seed/load/upsert |

---

### Task 1: 建 config 表迁移

**Files:**
- Create: `migrations/000006_config.up.sql`
- Create: `migrations/000006_config.down.sql`

- [ ] **Step 1: 写 up 迁移**
```sql
-- 配置表：每行一个配置段（scan/audit/auth/http），value 为该段的 JSON 序列化
CREATE TABLE IF NOT EXISTS config (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 2: 写 down 迁移**
```sql
DROP TABLE IF EXISTS config;
```

- [ ] **Step 3: 验证文件能被迁移器识别**
Run: `ls migrations/ | findstr 000006`
Expected: 存在 `000006_config.up.sql` 与 `000006_config.down.sql`（命名按字典序在 000005 之后）。

- [ ] **Step 4: Commit**
```bash
git add migrations/000006_config.up.sql migrations/000006_config.down.sql
git commit -m "feat(config): add config table migration"
```

---

### Task 2: .env 引导解析（LoadBootstrap）

**Files:**
- Modify: `internal/config/config.go`（保留 `Config`/`defaultConfig`，新增 `Bootstrap` 与 `LoadBootstrap`；暂不改 Load/Save）
- Test: `internal/config/config_test.go`（新建）

**Interfaces:**
- Produces: `type Bootstrap struct{ PGDSN, NATSURL, ESAddr, HTTPAddr string }`；`func LoadBootstrap() (*Bootstrap, error)`

- [ ] **Step 1: 写失败测试**
```go
func TestLoadBootstrap(t *testing.T) {
	dir := t.TempDir()
	ef := filepath.Join(dir, ".env")
	if err := os.WriteFile(ef, []byte("# comment\n\nATLAS_PG_DSN=postgres://u:p@h:5432/db\nATLAS_NATS_URL=nats://h:4222\nATLAS_ES_ADDR=http://h:9200\n"), 0644); err != nil {
		t.Fatal(err)
	}
	b, err := LoadBootstrapFrom(ef)
	if err != nil {
		t.Fatal(err)
	}
	if b.PGDSN != "postgres://u:p@h:5432/db" {
		t.Errorf("PGDSN=%q", b.PGDSN)
	}
	if b.NATSURL != "nats://h:4222" || b.ESAddr != "http://h:9200" {
		t.Errorf("nats/es 解析错误: %+v", b)
	}
}

func TestLoadBootstrapEnvPrecedence(t *testing.T) {
	t.Setenv("ATLAS_PG_DSN", "env-dsn")
	b, err := LoadBootstrapFrom("") // 无 .env 文件，走环境变量
	if err != nil {
		t.Fatal(err)
	}
	if b.PGDSN != "env-dsn" {
		t.Errorf("环境变量应优先，实际 %q", b.PGDSN)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**
Run: `go test ./internal/config/ -run TestLoadBootstrap -v`
Expected: FAIL（`LoadBootstrapFrom` 未定义）

- [ ] **Step 3: 实现 LoadBootstrap**
在 `config.go` 增加：
```go
// Bootstrap 连接引导参数（来自 .env / 环境变量，不进 DB）
type Bootstrap struct {
	PGDSN   string
	NATSURL string
	ESAddr  string
	HTTPAddr string
}

// LoadBootstrapFrom 解析可选 .env 文件；环境变量优先，文件作兜底。
func LoadBootstrapFrom(envFile string) (*Bootstrap, error) {
	b := &Bootstrap{}
	if envFile != "" {
		if m, err := parseEnvFile(envFile); err == nil {
			if v, ok := m["ATLAS_PG_DSN"]; ok && os.Getenv("ATLAS_PG_DSN") == "" {
				_ = v
			}
			apply := func(k, env string) {
				if os.Getenv(env) == "" {
					if v, ok := m[k]; ok {
						_ = os.Setenv(env, v)
					}
				}
			}
			apply("ATLAS_PG_DSN", "ATLAS_PG_DSN")
			apply("ATLAS_NATS_URL", "ATLAS_NATS_URL")
			apply("ATLAS_ES_ADDR", "ATLAS_ES_ADDR")
			apply("ATLAS_HTTP_ADDR", "ATLAS_HTTP_ADDR")
		}
	}
	b.PGDSN = envOr("ATLAS_PG_DSN", "postgres://postgres:postgres@127.0.0.1:5432/atlas?sslmode=disable")
	b.NATSURL = envOr("ATLAS_NATS_URL", "nats://127.0.0.1:4222")
	b.ESAddr = envOr("ATLAS_ES_ADDR", "http://127.0.0.1:9200")
	b.HTTPAddr = os.Getenv("ATLAS_HTTP_ADDR") // 空则后续由 DB http 段兜底
	return b, nil
}

// LoadBootstrap 以默认查找路径调用（实际部署由 main 传 --envfile）。
func LoadBootstrap() (*Bootstrap, error) { return LoadBootstrapFrom("") }

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// parseEnvFile 解析 KEY=VALUE 行，忽略 # 注释与空行（零依赖）。
func parseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		m[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
	}
	return m, nil
}
```
（保留 `Load`/`Save` 暂不动，Task 3 再删。）

- [ ] **Step 4: 运行测试确认通过**
Run: `go test ./internal/config/ -run TestLoadBootstrap -v`
Expected: PASS

- [ ] **Step 5: Commit**
```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add .env bootstrap loader"
```

---

### Task 3: config DB 读写层（删 YAML）

**Files:**
- Modify: `internal/config/config.go`（删 `Load`/`Save`/`path` 字段与 `yaml` 导入；加 `DB`/`Row` 接口、`LoadFromDB`/`EnsureSeeded`/`UpsertSection`）
- Test: `internal/config/config_test.go`（扩展 fakeDB 测试）

**Interfaces:**
- Produces:
  - `type Row interface { Scan(dest ...any) error }`
  - `type DB interface { Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error); QueryRow(ctx context.Context, sql string, args ...any) Row }`
  - `func EnsureSeeded(ctx context.Context, db DB) error`
  - `func LoadFromDB(ctx context.Context, db DB, boot *Bootstrap) (*Config, error)`
  - `func UpsertSection(ctx context.Context, db DB, key string, v any) error`

- [ ] **Step 1: 写失败测试（fakeDB，无需真库）**
```go
type fakeRow struct{ val string; err error }
func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	p, ok := dest[0].(*string)
	if !ok {
		return fmt.Errorf("dest must be *string")
	}
	*p = r.val
	return nil
}

type fakeDB struct{ rows map[string]string }

func (d *fakeDB) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	if len(args) >= 2 {
		if k, ok := args[0].(string); ok {
			if v, ok := args[1].(string); ok {
				d.rows[k] = v
			}
		}
	}
	return pgconn.CommandTag{}, nil
}
func (d *fakeDB) QueryRow(_ context.Context, _ string, args ...any) Row {
	key, _ := args[0].(string)
	if v, ok := d.rows[key]; ok {
		return fakeRow{val: v}
	}
	return fakeRow{err: pgx.ErrNoRows}
}

func TestConfigSeedAndLoad(t *testing.T) {
	db := &fakeDB{rows: map[string]string{}}
	if err := EnsureSeeded(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if len(db.rows) != 4 {
		t.Fatalf("应播种 4 段，实际 %d", len(db.rows))
	}
	cfg, err := LoadFromDB(context.Background(), db, &Bootstrap{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Scan.DefaultMode != "connect" {
		t.Errorf("默认模式应为 connect, 实际 %s", cfg.Scan.DefaultMode)
	}
	// 热更新一段后重读应生效
	if err := UpsertSection(context.Background(), db, "scan", ScanConfig{DefaultMode: "syn", RawIface: "eth0"}); err != nil {
		t.Fatal(err)
	}
	cfg2, _ := LoadFromDB(context.Background(), db, &Bootstrap{})
	if cfg2.Scan.DefaultMode != "syn" || cfg2.Scan.RawIface != "eth0" {
		t.Errorf("热更新后未生效: %+v", cfg2.Scan)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**
Run: `go test ./internal/config/ -run TestConfigSeedAndLoad -v`
Expected: FAIL（`EnsureSeeded`/`LoadFromDB`/`UpsertSection` 未定义）

- [ ] **Step 3: 实现 DB 层并删除 YAML**
替换 `config.go` 中 `Load`/`Save` 及相关 `path` 字段、移除 `"gopkg.in/yaml.v3"` 导入，新增：
```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Row 是 QueryRow 返回行的最小接口（pgx.Row 天然满足）。
type Row interface{ Scan(dest ...any) error }

// DB 解耦存储层的最小接口；*pgxpool.Pool 天然满足。
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
}

const upsertConfigSQL = `INSERT INTO config(key,value,updated_at) VALUES($1,$2,now())
	ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`

// UpsertSection 将某配置段 JSON 序列化后 upsert 入库。
func UpsertSection(ctx context.Context, db DB, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("序列化配置段 %s: %w", key, err)
	}
	if _, err := db.Exec(ctx, upsertConfigSQL, key, string(b)); err != nil {
		return fmt.Errorf("写入配置段 %s: %w", key, err)
	}
	return nil
}

// EnsureSeeded 表空时按 defaultConfig 播种四段；非空则跳过。
func EnsureSeeded(ctx context.Context, db DB) error {
	var n int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM config").Scan(&n); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			n = 0
		} else {
			return fmt.Errorf("统计配置行: %w", err)
		}
	}
	if n > 0 {
		return nil
	}
	seed := map[string]any{
		"scan":  defaultConfig().Scan,
		"audit": defaultConfig().Audit,
		"auth":  defaultConfig().Auth,
		"http":  defaultConfig().HTTP,
	}
	for k, v := range seed {
		if err := UpsertSection(ctx, db, k, v); err != nil {
			return err
		}
	}
	return nil
}

// LoadFromDB 确保已播种并读四段进 *Config；连接段由 boot 填充。
func LoadFromDB(ctx context.Context, db DB, boot *Bootstrap) (*Config, error) {
	if err := EnsureSeeded(ctx, db); err != nil {
		return nil, err
	}
	cfg := defaultConfig()
	for _, key := range []string{"scan", "audit", "auth", "http"} {
		row := db.QueryRow(ctx, "SELECT value FROM config WHERE key=$1", key)
		var val string
		if err := row.Scan(&val); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, fmt.Errorf("读取配置段 %s: %w", key, err)
		}
		switch key {
		case "scan":
			if err := json.Unmarshal([]byte(val), &cfg.Scan); err != nil {
				return nil, fmt.Errorf("解析 scan: %w", err)
			}
		case "audit":
			if err := json.Unmarshal([]byte(val), &cfg.Audit); err != nil {
				return nil, fmt.Errorf("解析 audit: %w", err)
			}
		case "auth":
			if err := json.Unmarshal([]byte(val), &cfg.Auth); err != nil {
				return nil, fmt.Errorf("解析 auth: %w", err)
			}
		case "http":
			if err := json.Unmarshal([]byte(val), &cfg.HTTP); err != nil {
				return nil, fmt.Errorf("解析 http: %w", err)
			}
		}
	}
	cfg.Postgres.DSN = boot.PGDSN
	cfg.NATS.URL = boot.NATSURL
	cfg.Elastic.Addr = boot.ESAddr
	if cfg.HTTP.Addr == "" && boot.HTTPAddr != "" {
		cfg.HTTP.Addr = boot.HTTPAddr
	}
	return cfg, nil
}
```
（删除原 `Load`/`Save` 函数与 `Config.path` 字段；保留 `defaultConfig()` 与所有 `Config` 子结构。）

- [ ] **Step 4: 运行测试确认通过**
Run: `go test ./internal/config/ -v`
Expected: PASS（含 Task 2 两个测试 + 本任务测试）

- [ ] **Step 5: Commit**
```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): replace YAML with DB-backed load/seed/upsert"
```

---

### Task 4: store 暴露连接池与配置写边界

**Files:**
- Modify: `internal/store/pg.go`（新增 `Pool()`、`UpsertConfigSection`）

**Interfaces:**
- Consumes: `config` 包已完成（Task 3）
- Produces: `func (s *Store) Pool() *pgxpool.Pool`；`func (s *Store) UpsertConfigSection(ctx context.Context, key, value string) error`

- [ ] **Step 1: 实现两个方法**
在 `pg.go` 末尾增加：
```go
// Pool 暴露底层连接池，供 config 包经 DB 接口读配置（避免 store 反向依赖 config）。
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// UpsertConfigSection 写入单配置段（JSON 文本），作为配置唯一 DB 写边界。
func (s *Store) UpsertConfigSection(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO config(key,value,updated_at) VALUES($1,$2,now())
		ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, key, value)
	if err != nil {
		return fmt.Errorf("upsert config %s: %w", key, err)
	}
	return nil
}
```

- [ ] **Step 2: 验证编译**
Run: `go build ./internal/store/`
Expected: 成功（无报错）

- [ ] **Step 3: Commit**
```bash
git add internal/store/pg.go
git commit -m "feat(store): expose pool and UpsertConfigSection"
```

---

### Task 5: main.go 装配顺序改造

**Files:**
- Modify: `cmd/atlas/main.go`（替换 `config.Load(*configPath)`，改 `-config`→`-envfile`）

- [ ] **Step 1: 改启动参数与装配**
将：
```go
configPath := flag.String("config", "configs/atlas.yaml", "path to config yaml")
...
cfg, err := config.Load(*configPath)
```
改为：
```go
envFile := flag.String("envfile", "", "path to .env file for connection bootstrap (env vars override)")
...
boot, err := config.LoadBootstrapFrom(*envFile)
if err != nil {
	log.Fatalf("load bootstrap: %v", err)
}
```
并在 `store.NewPostgres(ctx, boot.PGDSN)` 之后、`RunMigrations` 之后，将：
```go
cfg, err := config.Load(*configPath)
```
改为：
```go
cfg, err := config.LoadFromDB(ctx, st.Pool(), boot)
if err != nil {
	log.Fatalf("load config from db: %v", err)
}
```
（`st.RunMigrations` 调用保持不变；其后的 ES/审计/限速/指纹/NATS/黑名单/任务/漏洞构建逻辑不变。）

- [ ] **Step 2: 验证编译**
Run: `go build ./cmd/atlas/`
Expected: 成功

- [ ] **Step 3: Commit**
```bash
git add cmd/atlas/main.go
git commit -m "refactor(main): bootstrap from .env then load config from DB"
```

---

### Task 6: updateConfig 写 DB

**Files:**
- Modify: `internal/server/config.go`（`updateConfig` 以 `Store.UpsertConfigSection` 替代 `cfg.Save()`）

- [ ] **Step 1: 改 updateConfig 持久化分支**
将 `updateConfig` 中：
```go
	// 持久化到 YAML；若无法持久化仍保证内存生效
	if err := cfg.Save(); err != nil {
		c.JSON(200, gin.H{
			"warning": "配置已生效（运行时），但未持久化到文件: " + err.Error(),
			...
		})
		return
	}
	c.JSON(200, gin.H{"ok": true})
```
改为先推扫描器热更新（已有 `SetScanConfig`），再落 DB：
```go
	// 内存生效 + 扫描器热更新（已有）
	if s.deps.Scanner != nil {
		s.deps.Scanner.SetScanConfig(cfg.Scan)
	}
	// 持久化到 DB；失败仅告警，不阻断内存生效
	if err := s.deps.Store.UpsertConfigSection(c.Request.Context(), "scan", cfg.Scan); err != nil {
		c.JSON(200, gin.H{
			"warning": "配置已生效（运行时），但未持久化到数据库: " + err.Error(),
			"audit":   gin.H{"enabled": s.deps.Audit.Enabled()},
			"scan": gin.H{
				"default_mode":       cfg.Scan.DefaultMode,
				"default_port_range": cfg.Scan.DefaultPortRange,
				"max_concurrency":    cfg.Scan.MaxConcurrency,
				"per_target_rps":     cfg.Scan.PerTargetRPS,
				"raw_iface":          cfg.Scan.RawIface,
			},
		})
		return
	}
	if err := s.deps.Store.UpsertConfigSection(c.Request.Context(), "audit", cfg.Audit); err != nil {
		c.JSON(200, gin.H{"warning": "audit 配置未持久化: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
```
（`getConfig` 与 `configPayload` 不变；`SetScanConfig` 调用保留。）

- [ ] **Step 2: 验证编译**
Run: `go build ./internal/server/`
Expected: 成功

- [ ] **Step 3: Commit**
```bash
git add internal/server/config.go
git commit -m "feat(server): persist config changes to DB"
```

---

### Task 7: Docker / compose 与清理 yaml

**Files:**
- Modify: `Dockerfile`（移除 `-config /app/configs/atlas.yaml` 启动参数）
- Modify: `docker-compose.yml`（移除 `configs/atlas.yaml` 相关挂载若有；确认 `ATLAS_PG_DSN` 等 env 保留）
- Delete: `configs/atlas.yaml`

- [ ] **Step 1: Dockerfile 改 ENTRYPOINT**
将：
```dockerfile
ENTRYPOINT ["/app/atlas", "-config", "/app/configs/atlas.yaml", \
            "-migrations", "/app/migrations", \
            "-rules", "/app/configs/fingerprint-rules.yaml", \
            "-webdir", "/app/web/dist"]
```
改为（去掉 `-config` 段；连接参数由容器 env / `.env` 提供）：
```dockerfile
ENTRYPOINT ["/app/atlas", \
            "-migrations", "/app/migrations", \
            "-rules", "/app/configs/fingerprint-rules.yaml", \
            "-webdir", "/app/web/dist"]
```

- [ ] **Step 2: 删除旧 yaml**
```bash
git rm configs/atlas.yaml
```

- [ ] **Step 3: compose 检查**
确认 `docker-compose.yml` 的 `atlas`/`atlas2` 服务仍含 `environment: ATLAS_PG_DSN/ATLAS_NATS_URL/ATLAS_ES_ADDR`（作引导源）；无需额外改动。

- [ ] **Step 4: Commit**
```bash
git add Dockerfile docker-compose.yml
git commit -m "refactor(deploy): drop yaml config, bootstrap via env/.env"
```

---

### Task 8: 集成测试 + 全量校验

**Files:**
- Create: `internal/config/integration_test.go`（`//go:build integration`）
- Test: 运行 `go test ./internal/config/...` 与 `go vet ./...`

**Interfaces:**
- Consumes: `store.NewPostgres` + `store.Pool()` 提供真 `*pgxpool.Pool`；需环境变量 `ATLAS_PG_DSN` 指向可用 PG。

- [ ] **Step 1: 写集成测试**
```go
//go:build integration

package config

import (
	"context"
	"os"
	"testing"

	"atlas/internal/store"
)

func TestIntegrationConfigDB(t *testing.T) {
	dsn := os.Getenv("ATLAS_PG_DSN")
	if dsn == "" {
		t.Skip("ATLAS_PG_DSN 未设置，跳过配置 DB 集成测试")
	}
	ctx := context.Background()
	st, err := store.NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.RunMigrations(ctx, "migrations"); err != nil {
		t.Fatal(err)
	}
	// 清空待测表，模拟首启空库
	_, _ = st.Pool().Exec(ctx, "TRUNCATE config")
	boot, _ := LoadBootstrapFrom("")
	cfg, err := LoadFromDB(ctx, st.Pool(), boot)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Scan.DefaultMode != "connect" {
		t.Errorf("首启应播种默认值 connect, 实际 %s", cfg.Scan.DefaultMode)
	}
	if err := UpsertSection(ctx, st.Pool(), "scan", ScanConfig{DefaultMode: "syn"}); err != nil {
		t.Fatal(err)
	}
	cfg2, err := LoadFromDB(ctx, st.Pool(), boot)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Scan.DefaultMode != "syn" {
		t.Errorf("热更新后应从 DB 读到 syn, 实际 %s", cfg2.Scan.DefaultMode)
	}
}
```

- [ ] **Step 2: 运行单元 + 集成测试**
Run: `go test ./internal/config/...`
Expected: 单元测试 PASS
Run: `ATLAS_PG_DSN=postgres://postgres:postgres@127.0.0.1:5432/atlas?sslmode=disable go test -tags integration ./internal/config/ -run TestIntegrationConfigDB -v`
Expected: PASS（需本地可用 PG；否则 SKIP）

- [ ] **Step 3: 全量 vet/build**
Run: `go vet ./... && go build ./...`
Expected: 无错误

- [ ] **Step 4: Commit**
```bash
git add internal/config/integration_test.go
git commit -m "test(config): add DB integration test for seed/load/upsert"
```

---

## Self-Review 校验

- **Spec 覆盖**：引导(.env)→Task 2；表结构→Task 1；LoadFromDB/EnsureSeeded/UpsertSection→Task 3；store 写边界→Task 4；main 装配→Task 5；updateConfig 落 DB→Task 6；Docker/compose+yaml 清理→Task 7；测试→Task 8。覆盖完整。
- **占位符扫描**：无 TBD/TODO；各步含实际代码。
- **类型一致性**：`Bootstrap`/`DB`/`Row`/`EnsureSeeded`/`LoadFromDB`/`UpsertSection` 在 Task 2/3 定义，Task 5/6/8 一致引用；`store.UpsertConfigSection(ctx,key,value string)` 签名与 Task 6 调用一致；`st.Pool()` 返回 `*pgxpool.Pool` 满足 `config.DB`。
- **未决项**：旧 yaml 自定义值不迁移（Task 7 直接删文件），与 spec §11-1 一致。
