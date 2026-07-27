# 调度与任务（创建 / 周期 / 分解 / NATS 派发 / 断点续扫）

## Description
实现调度中心：创建一次性与周期扫描/漏洞任务，将目标分解为 `task_items` 持久化，发布到 NATS 主题由 Worker 池（本实例与他实例）消费；进度持久化以支持进程重启后断点续扫；支持优先级。

## Acceptance Criteria
- [ ] 创建一次性任务（scope / port_range / scan_mode / rate_limit），返回 task_id，状态 `pending` 进入调度
- [ ] 创建周期任务（cron 或"每 N 天/小时"），到点自动生成子任务，不影响历史记录；列表展示下次执行时间
- [ ] 任务分解为 `task_items`（status 0），发布到 NATS `atlas.scan` / `atlas.vuln` 队列组
- [ ] 进度持久化（total/done）；进程重启后从 `task_items` 续扫未完成任务，不重头
- [ ] 支持优先级字段，高优先级子任务先消费
- [ ] 多实例部署共享同一 NATS 与存储，自动横向扩展
- [ ] 类型检查与 lint 通过

## Dependencies
Issue #1

## Type
backend

## Priority
high

## SPEC Reference
2.3, 4.1, 5.1, 5.3
