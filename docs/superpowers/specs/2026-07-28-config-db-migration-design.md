# 配置系统 YAML → 数据库迁移 · 技术方案

日期：2026-07-28

## 0. 目标

将 atlas 的配置管理系统从「YAML 文件」全面迁移至「PostgreSQL」，并在以下条件下满足：
- 首次启动（或库内无配置）时自动用默认配置播种（seed）到 DB。
- 运行期完全废弃并移除对 YAML 配置文件的读取与依赖，统一经 DB 动态读写配置。
- 连接参数（DSN/NATS/ES）因鸡生蛋问题，改由 `.env`/环境变量提供，**不进 DB、不进 YAML**。

## 1. 总体架构（引导链）

```
.env / 环境变量 ──▶ LoadBootstrap() 解析出连接参数 (PG_DSN / NATS_URL / ES_ADDR / HTTP_ADDR)
        │
        ▼
   store.NewPostgres(DSN)  ──▶  RunMigrations()  (000006 建 config 表)
        │                                  │
        ▼                                  ▼
   config.LoadFromDB(db, boot)  ──▶  表空则 EnsureSeeded 写入默认四段
        │
        ▼
   内存 *Config：Scan/Audit/Auth/HTTP 来自 DB；Postgres/NATS/Elastic 来自 boot
        │
        ▼
   scan.New / server / 各组件 同现状消费 *Config
```

- DB 只存可热改的业务配置：`scan` / `audit` / `auth` / `http` 四段。
- `Config` 结构体保持不变；仅 `Postgres/NATS/Elastic` 三段改由 boot 填充；删除 `Save()`（YAML）。
- `http` 段放 DB，界面可改（与 scan 等一致）。

## 2. 数据库表结构

迁移 `migrations/000006_config.up.sql`（+ 对应 `.down.sql`）：

```sql
CREATE TABLE IF NOT EXISTS config (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,             -- 对应配置段的 JSON 序列化
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- `key` ∈ {`scan`, `audit`, `auth`, `http`}，每行一个段的 JSON。
- 读：`SELECT key, value FROM config`（一次取全量）。
- 写：`INSERT ... ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`（单行原子，热点仅 `scan` 段）。
- 种子行由 Go 端 `EnsureSeeded` 写入（不硬编码进 SQL）：
  - `scan`  = `defaultConfig().Scan` 的 JSON
  - `audit` = `defaultConfig().Audit` 的 JSON
  - `auth`  = `defaultConfig().Auth` 的 JSON（明文，见 §6）
  - `http`  = `defaultConfig().HTTP` 的 JSON

## 3. config 包 API 重构

文件：`internal/config/config.go`（删除 `yaml` 依赖与 `Load(path)`/`Save()`）。

新增函数：

- `LoadBootstrap() (*Bootstrap, error)`
  - 解析可选 `--envfile` 指向的 `.env`（`KEY=VALUE` 行，忽略 `#` 注释与空行，零新依赖手写解析）。
  - **环境变量优先**：先读 `os.Getenv("ATLAS_PG_DSN")` 等；若未设且 `.env` 提供则用 `.env` 值。
  - 返回 `{ PGDSN, NATSURL, ESAddr, HTTPAddr }`。
  - 缺失时回退到 localhost 默认值（与原 `defaultConfig` 行为一致）。

- `LoadFromDB(ctx, db DB, boot *Bootstrap) (*Config, error)`
  - 先 `EnsureSeeded(ctx, db)`。
  - `SELECT key,value FROM config` → 按 key 反序列化进 `cfg.Scan/Audit/Auth/HTTP`。
  - `cfg.Postgres.DSN = boot.PGDSN`、`cfg.NATS.URL = boot.NATSURL`、`cfg.Elastic.Addr = boot.ESAddr`、`cfg.HTTP.Addr` 来自 DB 的 `http` 段。
  - 返回 `*Config` 作为内存镜像（供 `getConfig`/各组件消费）。

- `EnsureSeeded(ctx, db DB) error`
  - `SELECT count(*) FROM config`；若 >0 直接返回。
  - 否则将 `defaultConfig()` 的 scan/audit/auth/http 各段 `json.Marshal` 后 `INSERT`。

- `UpsertSection(ctx, db DB, key string, v any) error`
  - `json.Marshal(v)` → `UPSERT` 对应段。

- `DB` 接口（最小依赖，避免 config 反向依赖 store）：
  ```go
  type DB interface {
      Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
      QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
  }
  ```
  `*pgxpool.Pool` 天然满足该接口。

## 4. store 包新增

文件：`internal/store/pg.go`

- `func (s *Store) Pool() *pgxpool.Pool`：暴露连接池给 config 读路径。
- `func (s *Store) UpsertConfigSection(ctx context.Context, key, value string) error`：封装 §2 的 UPSERT，作为唯一 DB 写边界（server 调用它而非 config 直写）。

## 5. 写路径（配置 API 热更新）

文件：`internal/server/config.go` 的 `updateConfig`：

- 保留：改内存 `s.deps.Cfg` + `s.deps.Scanner.SetScanConfig(cfg.Scan)`（运行时热更新，无需重启）。
- 将 `cfg.Save()` 替换为：
  ```go
  if err := s.deps.Store.UpsertConfigSection(ctx, "scan", cfg.Scan); err != nil { ... }
  // 以及 audit/auth/http 对应段
  ```
- `getConfig` 读内存镜像不变；`PUT` 全量回写语义保持，但落 DB。
- 持久化失败的 warning 文案相应调整（不再提「文件」）。

## 6. 认证字段处理

`auth` 段的 `password` / `secret` **明文入库**（与现状默认 `admin` / 固定 secret 一致，Q4 已确认）。DB 泄漏即暴露，属已知风险。

## 7. main.go 装配顺序调整

`cmd/atlas/main.go`：

```
boot, _ := config.LoadBootstrap()
st, _   := store.NewPostgres(ctx, boot.PGDSN)
st.RunMigrations(ctx, *migrationsDir)   // 建 config 表（000006）
cfg, _   := config.LoadFromDB(ctx, st.Pool(), boot)
scanner   := scan.New(st, limiter, defaultPorts, fp, cfg.Scan)   // 不变
srv      := server.New(server.Deps{Cfg: cfg, ...})               // 不变
```

- 启动参数 `-config` 改为 `-envfile`（可选；缺省仅从环境变量读取）。
- 其余依赖构建（ES、审计、限速、指纹、NATS、黑名单、任务、漏洞）逻辑不变，仅连接参数来自 `boot`。

## 8. Docker / compose

- 删除 `configs/atlas.yaml` 的挂载与启动参数 `-config`。
- `docker-compose.yml` 已通过 `environment:` 注入 `ATLAS_PG_DSN` 等 → 直接作为引导源；另可挂载 `.env` 作为非 compose 部署兜底。
- `configs/` 目录保留（fingerprint-rules / vuln templates 仍用，与配置无关，不动）。

## 9. 测试

- **单元**（`internal/config/config_test.go`，无需真库）：
  - `.env` 解析（含 `#` 注释、空行、环境变量优先覆盖）。
  - 四段 JSON 序列化↔反序列化 roundtrip（用 mock `DB` 或直接对 `defaultConfig` 段 marshal/unmarshal）。
- **集成**（`//go:build integration`）：起真 Postgres，验证
  - 「空库 → EnsureSeeded → 读出 == 默认值」；
  - 「改段 → UpsertSection → 重读生效」。

## 10. 文件改动清单

| 文件 | 改动 |
|---|---|
| `internal/config/config.go` | 删 YAML Load/Save；加 LoadBootstrap / LoadFromDB / EnsureSeeded / UpsertSection + `DB` 接口 |
| `migrations/000006_config.up.sql`(+down) | 建 `config` 表 |
| `internal/store/pg.go` | `Pool()` + `UpsertConfigSection` |
| `cmd/atlas/main.go` | 装配顺序：boot → 连库 → 迁移 → LoadFromDB；`-config` → `-envfile` |
| `internal/server/config.go` | `updateConfig` 写 DB 替代 `cfg.Save()` |
| `Dockerfile` / `docker-compose.yml` | 去 `-config`/yaml 挂载，保留/加 `.env` 引导 |
| `internal/config/config_test.go` | 单测 |

## 11. 风险与决策

1. **升级兼容（已定：不做 yaml→DB 导入）**：旧 `atlas.yaml` 的自定义值**不自动迁移**；升级后从默认值开始，界面可改。原因：一次性迁移代码是死代码、增加复杂度与出错面。
2. **并发写**：`UPSERT ... ON CONFLICT DO UPDATE` 单行原子，热点仅 `scan` 段，足够。
3. **secret 明文**：DB 泄漏即暴露，与现状一致（Q4 已定）。
4. **回滚**：若需回退到 YAML 方案，需同时恢复 `config.Load` 与 `-config` 参数（本次不保留）。
