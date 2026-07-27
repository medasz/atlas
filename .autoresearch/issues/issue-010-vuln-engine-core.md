# 漏洞引擎框架（自研 Go Check，仅验证不利用）

## Description
实现 Atlas 内置的 Go 原生漏洞检测框架，暴露 `Check(task)` 接口，支持多协议（http/dns/tcp）请求与模板化检测；坚持仅验证不利用，禁止任何写操作与利用载荷。设计参考 nuclei。

## Acceptance Criteria
- [ ] 暴露 `Check(task)` 接口，task 含 type(web/service) / netloc / target / meta
- [ ] 支持多协议请求（http / dns / tcp）
- [ ] POC 仅做存在性验证，禁止任何写操作 / 利用载荷（静态检查拒绝危险调用）
- [ ] 返回结构化结果：是否存在、漏洞名、等级、类型、参考、证明
- [ ] 模板执行在受限上下文运行，加载前静态校验
- [ ] 类型检查与 lint 通过

## Dependencies
Issue #8

## Type
backend

## Priority
high

## SPEC Reference
2.2, 5.1
