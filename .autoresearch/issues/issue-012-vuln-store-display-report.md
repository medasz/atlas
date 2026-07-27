# 漏洞存储 / 生命周期 / 展示 / 报表

## Description
实现漏洞检测任务的执行与结果管理：基于资产或检索条件发起检测，结果持久化带生命周期状态（open/fixed/recur），控制台列表/筛选与 Excel/CSV 导出。

## Acceptance Criteria
- [ ] 漏洞任务范围可选：全部资产 / 按检索条件筛选 / 指定目标；可选插件集（单个/全部/按类型）
- [ ] VulnEngine Worker 并发检测，受全局限速约束
- [ ] 漏洞记录含 asset_ref / kpid / cve / name / level / type / proof / status(open/fixed/recur) / first_found / last_verified
- [ ] 同资产同漏洞复验通过→open，不通过→fixed；去重 (asset_ref, kpid) 不重复插入
- [ ] 列表按等级（紧急/高危/中危/低危/提示）排序，支持按资产/类型/状态筛选
- [ ] 支持导出 Excel / CSV 报表
- [ ] 类型检查与 lint 通过；浏览器验证

## Dependencies
Issue #11

## Type
fullstack

## Priority
high

## SPEC Reference
3.1, 4.1
