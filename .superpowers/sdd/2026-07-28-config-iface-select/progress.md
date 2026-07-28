# SDD ledger — plan: docs/superpowers/plans/2026-07-28-config-iface-select.md

## 约束适配（用户：禁止自动 git commit）
- 不执行任何 `git commit`。所有改动保留在工作区未提交。
- 每个任务的 review package 用 `git diff <BASE> -- <本任务文件>` 生成（BASE=e0bb339f），文件互不重叠，可隔离。
- 实现由控制器直接落地（环境无通用 implementer 子代理；code-explorer 只读、code-reviewer 仅审阅）。审阅用 code-reviewer 子代理。

## BASE
- e0bb339f23b5216ee7235b29607b008c806fb7b6 (原始 HEAD，开始执行前)

## Todos
- Task 1: 后端 listInterfaces 处理器 + 路由 + 单测 (files: internal/server/config.go, internal/server/config_test.go) — DONE (build ok, TestListInterfaces PASS)
- Task 2: 前端 api.getInterfaces() (files: web/src/api.js)
- Task 3: Settings.vue 文本框改下拉框 (files: web/src/views/Settings.vue)
- Task 4: 构建与联调验证 (go build/test + web build) — DONE (go build ok, go test ok, npm run build ok)
- Final: code-reviewer 全分支审阅 — DONE (5/5 spec 合规，无 Critical/无阻塞 Important；已补 TestListInterfaces 的 Content-Type 断言，测试 PASS)

## 收尾
- 全部改动保留在工作区，未提交（用户约束：禁止自动 git commit）。
- 待用户许可后提交：config.go / config_test.go / api.js / Settings.vue + 计划与规格文档。
