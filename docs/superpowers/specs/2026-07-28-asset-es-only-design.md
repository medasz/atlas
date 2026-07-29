# 资产数据全面迁移至 Elasticsearch（ES 唯一存储 · 方案 B）

- 日期：2026-07-28
- 状态：已评审（待写实现计划）
- 决策（brainstorming 确认）：
  1. **ES 唯一存储**：移除 PG 的 `hosts` / `ports` / `domains` 表，资产只存 ES。
  2. **仅资产本体**：hosts/ports/domains 迁 ES；vulns / tasks / blacklist / config 仍留 PG。
  3. **备份恢复**：ES 官方快照（snapshot repository 挂持久卷）+ 启动自动恢复。
  4. **结构选型 B**：抽取 `AssetStore` 接口，由 `esasset` 包以 ES 后端实现，与 PG `Store` 解耦。

## 1. 背景

当前架构为「PG 权威源 + ES 搜索索引」双写：`UpsertHost/Port/Domain` 先写 PG 再同步 ES，检索 `SearchAssets` 优先走 ES、ES 出错才回退 PG。

排查发现（2026-07-28）：ES `assets` 索引仅含 2 个 host 文档、0 个 port 文档，而 PG `ports` 表有 6000 行、`port=22` 有 1 行，`es_pending=0`。根因是端口数据从未进入 ES，而 ES 优先检索在索引不完整时**静默返回空**，导致检索无数据。

本次目标：资产本体全面、可靠地存储于 ES，检索直接且唯一地走 ES，并具备快照级可恢复能力。

## 2. 目标架构

```
                ┌─────────────── 写 ───────────────┐
 scan worker ───┤                                ├──▶ ESAssetStore ──▶ Elasticsearch (assets)
                │  AssetStore 接口（依赖倒置）      │   (esasset 包)
 admin reindex ─┤                                └──────────────────────────────────────┐
                └─────────────── 读 ───────────────┘                                  │
                                                                                     │
 HTTP 资产接口 ──▶ AssetStore（资产）                                                 │
 HTTP 漏洞接口 ──▶ PG Store（vulns / tasks / blacklist / config）◀── vuln engine      │
                                      │                                              │
                                      └── vulns.asset_ref 字符串关联资产（PG 内）─────┘
```

- **资产读写**全部经 `AssetStore` 接口，唯一实现为 `esasset.ESAssetStore`（基于 `ESClient`）。
- **PG `Store`** 仅保留 vulns / tasks / blacklist / config 等非资产数据；`getHostDetail` 由 HTTP 层组合：资产部分走 `AssetStore`、漏洞部分走 PG `Store`（`asset_ref` 字符串关联，无需 FK）。
- 去除所有 PG 资产双写与 `es_pending` 重试机制。

## 3. 数据模型（ES，沿用现有）

- 索引 `assets`，映射 `assetMapping` 不变（doc_type / ip / port / proto / service / version / banner / title / host / name / registrable_domain / server / tech / os / asn / org / geo / is_ipv6 / last_seen）。
- `_id` 规则沿用现状：`host:<ip>`、`port:<ip>:<port>`（proto 维度当前未进 `_id`，与现有 `UpsertPort` 行为一致，本期不改变）、`domain:<name>`。
- ES 文档的构建逻辑（原 `pg.go` `UpsertHost/Port/Domain` 内的 doc 组装）**搬入 `esasset` 包**，由 ES 后端独占。

## 4. AssetStore 接口

定义在 `internal/asset/asset.go`（或 `internal/store/assetstore.go`）：

```go
type AssetStore interface {
    UpsertHost(ctx context.Context, h model.Host) error
    UpsertPort(ctx context.Context, p model.Port) error
    UpsertDomain(ctx context.Context, d model.Domain) error
    GetHost(ctx context.Context, ip string) (model.Host, error)        // 未找到返回 ErrNotFound
    ListPortsByIP(ctx context.Context, ip string) ([]model.Port, error)
    ListDomains(ctx context.Context) ([]model.Domain, error)
    GetHostDetail(ctx context.Context, ip string) (model.Host, []model.Port, error)
    SearchAssets(ctx context.Context, q, docType string, page, pageSize int) (*store.SearchResult, error)
}
```

`store.SearchResult` 仍置于 `store` 包（esasset 引用它，无循环依赖：esasset 依赖 store，store 不依赖 esasset）。

## 5. 写入链路

- `esasset.ESAssetStore.Upsert*`：组装 ES doc → `es.IndexAsset(id, doc)`。`_id` upsert 幂等。
- **写失败即返回 error**（ES 是唯一源，禁止静默丢弃）。scan worker 当前以 `_ =` 忽略返回值，改为记录日志 + 失败计数（可选单次重试，重试后仍失败则记日志，不阻塞整任务）。
- 删除 `store` 包内的 `indexAsset` / `FlushPendingES` 及 `main.go` 中每 30s 的重试 ticker。

## 6. 读取链路

`ESClient` 新增方法：`Get(ctx, id) (map[string]any, error)`（404→`ErrNotFound`）、`Delete(ctx, id) error`。

- `GetHost(ip)` → `es.Get("host:"+ip)` → 映射 `model.Host`；404→`ErrNotFound`（HTTP 层转 404）。
- `ListPortsByIP(ip)` → ES 查 `doc_type:port AND ip:<ip>`，按 `port` 升序。
- `ListDomains` → ES 查 `doc_type:domain`，**分页/滚动**拉取（避免一次性返回全量），默认按 `last_seen` 倒序。
- `GetHostDetail(ip)` → `GetHost` + `ListPortsByIP`；漏洞由 HTTP 层另查 PG `ListVulnsByHost`。
- `SearchAssets` → `buildESQuery` + `es.Search`，**删除 PG union 回退**（`searchAssetsPG` / `scopeUnionSelect` / `assetCols` / `renumber` 中仅资产使用的部分一并移除）。

## 7. 迁移与回填

- 新增 `ReindexFromPG(ctx, pg *Store, asset AssetStore)`：用 PG `Store` 的既有资产读方法（过渡期仍保留）全量读取 hosts/ports/domains，经 `AssetStore` 写入 ES。
- 暴露 `POST /api/admin/reindex`（仅管理员）触发。
- **执行顺序（关键，错序即丢数据）**：
  1. 部署新代码（资产读写已走 `AssetStore`/ES；PG 资产表与 PG 资产读方法仍临时保留）。
  2. 调用 `POST /api/admin/reindex`，将 PG 资产灌入 ES。
  3. 校验 ES 文档数 ≈ PG 资产数。
  4. 执行删表迁移 `0000XX_drop_asset_tables.up.sql`（`DROP TABLE hosts, ports, domains;`；`es_pending` 列随表删除）。
  5. 移除 PG `Store` 中资产方法、`ReindexFromPG` 依赖的 PG 资产读方法、`/admin/reindex` 端点（或保留为 no-op 并注明已下线）。
- `vulns.asset_ref` 为字符串、无外键，删表安全。

## 8. 备份 / 恢复（ES 快照）

- `docker-compose.yml` 给 `elasticsearch` 服务增加持久卷 `es_backup` 挂载到 `/backups`，并在 ES 配置中设 `path.repo: ["/backups"]`。
- `main.go` 启动流程：`CreateIndex` 后注册快照仓库 `PUT _snapshot/atlas_backup`（已存在则忽略）；若 `assets` 索引文档数为 0 且仓库存在最新快照 → 自动 `restore`（覆盖空索引）。
- 后台 ticker（默认每 6h，可配置）对 `assets` 索引打快照。
- README / 运维文档补充：手动恢复命令、卷备份说明。

## 9. 移除项清单

- PG：表 `hosts` / `ports` / `domains`（及 `es_pending` 列）；`UpsertHost/Port/Domain`、`GetHost`、`ListPortsByIP`、`ListDomains` 的 PG 实现；`searchAssetsPG`、`scopeUnionSelect`、`assetCols`、仅资产用的 `renumber`/`likeExpr` 分支；`indexAsset`、`FlushPendingES`、`es_pending` 相关 SQL。
- 启动：`main.go` 中每 30s 的 `FlushPendingES` ticker。
- `000002_es_pending` 迁移标记为历史（保留文件，不再新建该列）。

## 10. 测试

- 保留：`query.go` 的 `toPG` / `toES` 解析单测（纯函数，与存储无关）。
- 新增：`esasset` 包单测，用 `ESClient` 接口 mock 或测试用 ES 容器验证 Upsert/Get/ListPortsByIP/Search 行为。
- 冒烟（手动）：reindex 后验证 `port=22`、`ip=...`、主机详情（资产+漏洞）、列表分页与排序均正常；ES 容器重建后能从快照自动恢复。

## 11. 风险与缓解

- **迁移错序丢数据**：以「先 reindex 校验、后删表」为强制步骤，并在删表迁移中加注释强调顺序；建议 reindex 与删表由人工分步执行而非自动连跑。
- **ES 单点**：靠第 8 节快照 + 持久卷缓解；不在本期引入 ES 多节点（超出 MVP 范围）。
- **ListDomains 全量**：改用分页/滚动，避免大结果集撑爆内存。
- **写失败语义变化**：scan worker 需适配错误（日志+计数），避免 panic 或静默丢失。

## 12. 实现顺序（建议）

1. 定义 `AssetStore` 接口 + `esasset.ESAssetStore`（搬入 doc 构建、实现读写/搜索）。
2. `ESClient` 增加 `Get` / `Delete`。
3. 重写/调整 `scan.go`、`server/asset.go`、`main.go` 依赖注入，注入 `AssetStore`。
4. `getHostDetail` 改为组合 `AssetStore` + PG vulns。
5. 删除 PG 资产双写与 `es_pending` / `FlushPendingES` / 30s ticker。
6. 加 `ReindexFromPG` + `/api/admin/reindex`；写实删表迁移。
7. docker-compose 加 `es_backup` 卷 + `path.repo`；`main.go` 加快照注册/自动恢复/ticker。
8. 测试 + 冒烟。
