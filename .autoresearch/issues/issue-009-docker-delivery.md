# Docker 交付（Dockerfile + docker-compose）

## Description
提供多阶段 Dockerfile 构建单二进制，以及 docker-compose 编排 atlas + nats + postgres + elasticsearch，便于多实例共享存储横向扩展。

## Acceptance Criteria
- [ ] 多阶段 Dockerfile 构建 atlas 单二进制镜像
- [ ] docker-compose.yml 编排 atlas + nats + postgres + elasticsearch 四项服务
- [ ] 启动后服务可运行；启动多份 atlas 实例共享同一 PG/ES/NATS 协同工作
- [ ] 镜像构建通过，容器启动后健康检查可用
- [ ] 类型检查与 lint 通过（构建通过）

## Dependencies
Issue #8

## Type
infra

## Priority
mid

## SPEC Reference
2.4, FR-20
