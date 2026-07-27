# 可编辑黑名单（录入 / 批量导入 / 扫描前过滤）

## Description
实现合规拦截的核心——可编辑黑名单。支持手动录入与文件/API 批量导入 IP/CIDR/域名三类"不扫描的资产"；在扫描任务派发前按黑名单过滤目标，命中项不进入扫描队列并记录审计（动作=excluded）。CIDR 须校验格式。

## Acceptance Criteria
- [ ] 支持录入 IP / CIDR / 域名三种类型黑名单项，同值去重不重复写入
- [ ] 提供 `GET /api/v1/blacklist` 列表（类型、值、录入人、时间）
- [ ] 提供 `POST /api/v1/blacklist` 新增单条
- [ ] 提供 `DELETE /api/v1/blacklist/:id` 删除
- [ ] 提供 `POST /api/v1/blacklist/import` 批量导入，返回成功/失败计数与逐条错误（行号+原因），合法行仍入库
- [ ] 扫描派发前按黑名单过滤，命中项不进入扫描队列，记录审计（动作=excluded），其余目标正常执行
- [ ] CIDR 经 `net.ParseCIDR` 校验，域名/IP 规范化后存储
- [ ] 类型检查与 lint 通过

## Dependencies
Issue #1

## Type
backend

## Priority
high

## SPEC Reference
2.2, 4.1, 5.2, 7.3
