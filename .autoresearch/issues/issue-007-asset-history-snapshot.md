# 资产变更快照 / 历史对比

## Description
保留资产历史快照，支持按时间对比发现新增/下线/变更的资产。

## Acceptance Criteria
- [ ] 资产字段变更时保留旧值快照（asset_history：entity_type/entity_key/del_time/type=update/snapshot）
- [ ] 提供按时间对比接口，返回新增 / 消失 / 变更的端口或服务
- [ ] 类型检查与 lint 通过

## Dependencies
Issue #6

## Type
backend

## Priority
mid

## SPEC Reference
3.1, 5.4
