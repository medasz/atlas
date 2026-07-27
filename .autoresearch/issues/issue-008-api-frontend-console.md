# REST API + Vue3/Element Plus 前端控制台

## Description
实现 Atlas 的 REST API（Gin）与 Vue3 + Element Plus 前端控制台，覆盖黑名单管理、任务管理、资产检索三大模块；管理员 JWT 登录；登录后强制展示法律警示横幅。

## Acceptance Criteria
- [ ] Gin 路由 + handler：blacklist / tasks / assets / audit / config / auth（阶段一范围）
- [ ] 管理员 JWT 登录（`POST /api/v1/auth/login`），非登录访问返回 401
- [ ] Vue3 + Element Plus 前端构建并接入 REST；黑名单管理页、任务管理页、资产检索页可用
- [ ] 登录后首页强制展示法律警示横幅（提示未经授权扫描第三方资产可能违法）
- [ ] 统一错误结构 `{error:{code,message}}`，错误码见 SPEC §4.3
- [ ] 类型检查与 lint 通过；浏览器验证

## Dependencies
Issue #2, Issue #3, Issue #4, Issue #5, Issue #6

## Type
fullstack

## Priority
high

## SPEC Reference
4.1, 7
