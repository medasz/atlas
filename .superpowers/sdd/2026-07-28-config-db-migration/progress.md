# SDD ledger — plan: docs/superpowers/plans/2026-07-28-config-db-migration.md

## Tasks
- Task 1: complete (commit 84d8584, review clean) — 建 config 表迁移 000006
- Task 2: complete (commit 8922604, review clean) — .env 引导解析 LoadBootstrap/LoadBootstrapFrom
- Task 3: complete (UNCOMMITTED, 用户暂停 commit) — config DB 读写层（删 YAML/Load/Save）；修正：新增 PoolDB 适配器解决 *pgxpool.Pool 不满足 config.DB 接口（QueryRow 返回类型不匹配）
- Task 4: complete (UNCOMMITTED) — store.UpsertConfigSection（Pool() 已存在）
- Task 5: complete (UNCOMMITTED) — main.go -envfile + LoadFromDB 装配
- Task 6: complete (UNCOMMITTED) — updateConfig 写 DB 替代 cfg.Save()；修复未使用 context import
- Task 7: complete (UNCOMMITTED) — Docker ENTRYPOINT 去 -config；删除 configs/atlas.yaml
- Task 8: complete (UNCOMMITTED) — integration_test.go + go build ./cmd/atlas/ 通过 (BUILD=0)

## 计划偏差（实现优于计划）
- PoolDB 适配器：计划/spec 声称 `*pgxpool.Pool` 天然满足 `config.DB`，实际因 `DB.QueryRow` 返回 `Row` 接口而 `*pgxpool.Pool.QueryRow` 返回 `pgx.Row`（返回类型不同）而不满足。实现新增 `PoolDB` 适配器；main 与 integration_test 均用 `config.NewPoolDB(st.Pool())`。

## Final Review (code-reviewer, 2026-07-28)
- Spec 合规：8/8 ✅
- Critical：无
- Important：
  - I1: updateConfig 并发写 *Config 无锁（既有设计，本次把内存提升为唯一事实源+DB 回写后放大风险）→ 建议落库成功后再回写内存或加锁
  - I2: updateConfig 仅持久化 scan/audit，未持久化 http/auth（属计划既定范围：configPayload 仅含 scan/audit；auth/http 界面编辑非本次迁移目标）
- Minor：双写路径分散、.env 解析失败静默忽略、os.Setenv 副作用、README 未同步、spec/plan "天然满足"描述应更正为"经 PoolDB 适配"
- 结论：无阻断性问题，可合并

