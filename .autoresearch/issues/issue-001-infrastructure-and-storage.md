# 基础设施与存储骨架

## Description
搭建 Atlas 单二进制的底层基础设施：配置加载、PostgreSQL 仓储、NATS 封装、令牌桶限速、可开关审计模块。为后续所有组件（黑名单/调度/扫描/指纹/漏洞/API）提供统一依赖。数据库表结构按 SPEC §3.1 的 DDL 通过 golang-migrate 首次启动自动建表。

## Acceptance Criteria
- [ ] 提供配置加载（YAML/env）：限速默认值、审计开关、扫描默认（scan_mode/port_range）、NATS/PostgreSQL/Elasticsearch 连接
- [ ] 提供 PostgreSQL 仓储层，封装 hosts/ports/domains/blacklist/tasks/task_items/vulns/audit_logs/templates/config 表读写
- [ ] 提供 golang-migrate 迁移脚本（migrations/*.up.sql + .down.sql），首次启动自动执行建表
- [ ] 提供 NATS 封装（发布 / 队列组订阅），支持 `atlas.scan` 与 `atlas.vuln` 主题
- [ ] 提供令牌桶限速（x/time/rate）：全局桶 + 每目标桶，超额请求排队
- [ ] 提供审计模块，受开关控制；开关关闭时所有审计写入被短路
- [ ] 类型检查与 lint 通过

## Dependencies
None

## Type
backend/infra

## Priority
high

## SPEC Reference
2.2, 3.1, 3.5
