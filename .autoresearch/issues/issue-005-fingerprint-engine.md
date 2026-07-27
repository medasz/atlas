# 指纹识别引擎（规则 + wappalyzergo）

## Description
实现指纹识别规则引擎，对 banner/响应头/body/title 识别服务、组件、版本、CMS。识别维度与签名思路参考 WhatWeb / wappalyzergo / CMSeeK；优先集成 wappalyzergo（Go 原生库）复用其签名集，自研规则以结构化文件管理并支持热加载。

## Acceptance Criteria
- [x] 规则以结构化文件（yaml）组织，维度以规则名表达 service / component / version / cms
- [x] 规则支持匹配 响应头 / body（含 title）/ TCP banner（新增 `banner_re`）
- [x] 指纹维度覆盖 HTTP 响应头、Set-Cookie（header 规则）、body 特征、`<meta generator>`（body 规则）、特定路径/文件（HTTP 请求模板另见漏洞引擎）、TLS 证书（wappalyzergo 已覆盖；原始证书存 `ports.cert`）
- [x] 集成 wappalyzergo（Go 库）复用其签名集
- [x] 提供热加载接口（`POST /api/fingerprint/reload` + SIGHUP 信号）；更新规则无需重启
- [x] 识别结果写入资产 `webinfo.tech`（component/tag），并保留 service/version 列；前端详情可展示
- [x] 类型检查与 lint 通过（`go build` / `go vet` 通过，`internal/fingerprint` 单测含 banner/header/body/社区库用例）

## Dependencies
Issue #4

## Type
backend

## Priority
high

## SPEC Reference
2.2, 5.1
