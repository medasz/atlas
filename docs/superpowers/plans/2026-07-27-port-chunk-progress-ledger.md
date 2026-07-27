# SDD ledger — plan: docs/superpowers/plans/2026-07-27-port-chunk-progress.md

> 环境说明：atlas 目录不是 git 仓库，故跳过 git commit / worktree / review-package 环节。
> 改用：直接实现 + `go build` 逐任务验证 + 本 ledger 记录进度。
> 提交需用户明确许可后再做（git init 或由用户决定）。

## Tasks
- [x] Task 1: config port_chunk_size (build OK)
- [x] Task 2: migration 000004 (up/down sql)
- [x] Task 3: model TaskItem.Ports (build OK)
- [x] Task 4: store 适配 ports (vet OK)
- [x] Task 5: TaskMsg.Ports (注意: TaskMsg 实际在 task.go 而非 queue/nats.go，已修正位置)
- [x] Task 6: Processor 接口 + scan/vuln/noop 签名 (build OK)
- [x] Task 7: 调度全链路透传 + 切块 (tests PASS, build OK)
- [x] Task 8: main.go 装配 (build OK)
- [x] Task 9: 集成验证 (docker) — API: progress.total=66; DB: 66 行 ports=1-1000..65001-65535; 实时递增受长命令守卫限制未观察，但代码路径已验证
