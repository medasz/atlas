# 声明式检测模板（nuclei 风格 YAML）+ aider 带外

## Description
实现 nuclei 风格的声明式检测模板体系（request + matchers + extractors，支持 http/dns/tcp 多协议），并支持带外辅助验证（aider）用于无回显漏洞。

## Acceptance Criteria
- [ ] 模板含 target / request（支持 http / dns / tcp 等协议）/ matchers（word / regex / binary / status / size 等）/ extractors
- [ ] 引擎按 matchers 判定命中并返回标签 / 证据
- [ ] 提供 `GetPlugins()` 列出全部已加载模板
- [ ] aider：支持配置地址（http://ip:8088）
- [ ] 提供 DNS 触发（nslookup randomstr ip）+ 验证（返回 YES）机制
- [ ] 提供 HTTP 触发（add/randomstr）+ 验证（check/randomstr 返回 YES）机制
- [ ] 类型检查与 lint 通过

## Dependencies
Issue #10

## Type
backend

## Priority
high

## SPEC Reference
4.1, 5.1
