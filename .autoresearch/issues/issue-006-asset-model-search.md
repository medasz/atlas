# 资产建模与检索（PG 持久化 + ES 索引 + 检索 API/控制台）

## Description
将扫描结果落成统一资产模型存入 PostgreSQL，同步索引到 Elasticsearch；提供类巡风的检索语法（ip/port/server/title/tag/banner/hostname/all）查询与 Web 控制台列表/详情。

## Acceptance Criteria
- [x] 资产模型 Host / Port / Domain / Cert 持久化 PG，按 (ip, port) 冲突更新并保留 first/last_seen
- [x] 每次端口行 upsert 同步写/更新 ES 文档（`_id=ip:port`，映射见 SPEC §3.2）
- [x] 检索支持类 FOFA 的 Dork 表达式语法：`field=value/==/!=/*=`（包含/精确/排除/模糊）、`&&`/`||` 逻辑、`()` 分组优先级；字段覆盖 ip/port/protocol(base_protocol)/server/banner/title/os/org/asn/app/product/host/domain/body/header/country/region/city/cert/is_ipv6 等，裸词走全字段全文检索；ES 与 PG 兜底均支持
- [x] 按 FOFA 完整字段表把 `host`/`domain`/`is_ipv6`/`protocol(base_protocol)` 落到具体列：`ports.host` 文本列、`hosts.is_ipv6`/`ports.is_ipv6` 布尔列、`domains` 表（name/registrable_domain/resolved_ips/cname/org/asn/is_ipv6/whois），新增 `type=domain` 检索入口，`is_ipv6` 用 term/布尔比较、`host`/`domain` 在 port 作用域映射到 `host` 列、在 domain 作用域映射到 `domains.name`
- [x] 千万级数据下分页响应 P95 < 1s（ES 全文/term 检索，已在映射与查询中按字段索引）
- [x] ES 不可达时降级为 PG 兜底（键式 JSONB/ILIKE），资产不丢；索引失败计入内存待补队列并置 `es_pending`，后台每 30s 重试成功后清除
- [x] Web 控制台列表（IP/端口/服务/组件/标题/最近发现时间，含域名类型与全字段提示）+ 详情（`GET /api/hosts/:ip/detail` 返回主机 + 全部端口（含 host/is_ipv6/指纹 tech/server）+ 关联漏洞，前端弹窗表格化展示）
- [x] 类型检查与 lint 通过；单测覆盖键式解析器（`internal/store`）。浏览器验证需本地 `npm run dev` 启动前端

## Dependencies
Issue #4, Issue #5

## Type
fullstack

## Priority
high

## SPEC Reference
3.1, 3.2, 4.1
