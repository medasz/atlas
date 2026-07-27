# Kunpeng POC → 模板迁移

## Description
将 Kunpeng 存量 POC 内容迁移到 Atlas 新模板体系：35 个 JSON 插件转换为 nuclei 风格的新模板格式（检测逻辑一致），51 个 Go 插件端口化到新框架（MVP 先迁移高危/常用部分）。

## Acceptance Criteria
- [ ] 35 个 Kunpeng JSON 插件转换为新模板格式，检测逻辑保持一致
- [ ] 51 个 Kunpeng Go 插件端口化到新框架（MVP 先迁高危/常用）
- [ ] 迁移后的模板可被 `GetPlugins()` 列出、被 `Check` 命中
- [ ] 类型检查与 lint 通过

## Dependencies
Issue #11

## Type
backend

## Priority
mid

## SPEC Reference
3.3
