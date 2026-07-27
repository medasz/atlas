# SPEC：互联网资产与漏洞测绘平台（Atlas MVP）

> 技术规格说明书，派生自：`tasks/prd-asset-vuln-mapping-mvp.md`
> 生成日期：2026-07-27 ｜ 目标分支：`main` ｜ 状态：待评审

## 1. 概述

### 1.1 本 SPEC 覆盖范围
本 SPEC 定义 Atlas（互联网资产与漏洞测绘平台）MVP 的技术实现方案：单二进制 Go 服务，内置资产测绘引擎（端口/HTTP 探测 + 指纹识别）与漏洞检测引擎（nuclei 风格模板），通过可编辑黑名单做合规拦截、可手动配置限速、可开关审计，使用 PostgreSQL 持久化 + Elasticsearch 检索 + NATS 跨实例任务分发。覆盖范围含 PRD 全部 21 个用户故事，按两阶段交付。

### 1.2 PRD 引用
- 来源：`tasks/prd-asset-vuln-mapping-mvp.md`
- 用户故事：US-001 ~ US-021
- 功能需求：FR-1 ~ FR-20
- 非目标（本期不做）：被动数据源、拓扑大屏、K8s 微服务、多租户、CMDB 对接、漏洞利用

### 1.3 设计决策汇总
| 决策项 | 选择 | 理由 |
|--------|------|------|
| 部署形态 | 单二进制，多实例共享 NATS/PG/ES | OQ-1 确认，满足百万级且不引入 K8s |
| 内部通信 | 进程内函数调用 + NATS 仅跨实例分发 | 单二进制最简洁；NATS 做实例间负载均衡 |
| 检索存储 | MVP 即用 Elasticsearch | OQ-0 确认，检索能力最强 |
| 前端 | Vue 3 + Element Plus（或 Ant Design Vue） | OQ-2 确认 |
| 限速默认值 | 单实例全局并发 500、每目标 10 req/s（可调） | OQ-3 建议值 [假设] |
| 漏洞引擎 | 自研 Go 引擎，nuclei 风格 YAML 模板 | OQ-4，复用社区签名且深度集成 |
| 交付阶段 | 阶段一资产测绘主线，阶段二漏洞引擎主线 | OQ-2 交付范围，降低首版风险 |
| 黑名单 | 默认放行，扫描前排除黑名单内目标 | 用户决议 |
| 审计 | 可开关，禁用时不写记录 | 用户决议 |

---

## 2. 架构

### 2.1 系统上下文
```
                         ┌──────────── 可编辑黑名单（排除项）────────────┐
                         │  (Web/API 录入 IP/网段/域名)                  │
                         └──────────────────┬──────────────────────────┘
                                            │ 过滤
        ┌─────────────── Atlas 单二进制实例 ───────────────┐
        │  API(Gin) ── 配置/任务/资产/漏洞/审计 接口         │
        │     │                                          │
        │  Scheduler ──分解任务→发布 NATS 主题             │
        │     │                          │               │
        │  Worker 池（进程内 goroutine）│                │
        │   ├─ Scanner（多模式端口+HTTP 探测）            │
        │   ├─ Fingerprinter（规则+wappalyzergo）         │
        │   └─ VulnEngine（nuclei 风格模板，仅验证）      │
        │     │                                          │
        │  Store：PostgreSQL（资产/任务/漏洞/审计）        │
        │  Search：Elasticsearch（资产检索）              │
        └──────────────────────────────────────────────────┘
                     │  NATS 主题（跨实例分发）   │
                ┌────┴────┐                  ┌───┴────┐
                │ 实例 B   │  ... 共享 PG/ES   │ 实例 N  │
                └─────────┘                  └─────────┘
```
多实例通过共享 NATS 主题 + 队列组实现负载均衡；各实例进程内组件以函数调用协作。

> 术语说明：一个"**实例**"指一份独立运行的 atlas 二进制进程；"**跨实例**"指多份进程之间经 NATS 做任务负载均衡（即横向扩展）。同一实例内部，Scheduler/Scanner/Fingerprinter/VulnEngine/API 等组件以 Go 函数调用协作，**不走 NATS**；NATS 仅用于把任务分发给不同实例的 Worker 池。

### 2.2 组件设计
| 组件 | 包路径 | 职责 | 边界 |
|------|--------|------|------|
| Config | `internal/config` | 加载 YAML/env 配置（限速、审计开关、扫描默认、NATS/PG/ES 连接） | 全局单例 |
| Blacklist | `internal/blacklist` | 黑名单 CRUD、命中判断（IP/CIDR/域名） | 仅过滤，不存扫描结果 |
| Store | `internal/store` | PostgreSQL 仓储 + ES 写入/查询 | 屏蔽 DB 细节 |
| Queue | `internal/queue` | NATS 发布/订阅封装，队列组消费 | 仅跨实例任务分发 |
| Scheduler | `internal/scheduler` | 任务分解、派发、进度持久化、断点续扫、周期触发 | 依赖 Queue/Store/Blacklist |
| Scanner | `internal/scanner` | 端口多模式探测 `Probe()` + HTTP 探测 | 依赖 RateLimit/Blacklist 结果驱动 |
| Fingerprinter | `internal/fingerprinter` | 规则引擎 + wappalyzergo 集成 | 输入 banner/header/body |
| VulnEngine | `internal/vuln` | 模板加载、Check、aider 带外 | 仅验证不利用 |
| RateLimit | `internal/ratelimit` | 令牌桶（全局 + 每目标） | 所有出站请求经此 |
| Audit | `internal/audit` | 审计写入（受开关控制） | 独立表 |
| API | `internal/api` | REST 接口 + 静态前端 | 唯一外部入口 |
| Model | `internal/model` | 领域结构体 | 共享 |

### 2.3 模块交互
**资产测绘数据流**：API 创建任务 → Scheduler 校验黑名单并分解目标为 `task_items` → 发布 `atlas.scan` → 本/他实例 Worker 消费 → Scanner 端口探测 → Fingerprinter 识别 → 结果写 Store（PG）+ 索引 ES → 进度更新；完成置 `done`。
**漏洞检测数据流**：任务 `kind=vuln` → 发布 `atlas.vuln` → VulnEngine 按资产/模板检测 → 结果写 `vulns` → 生命周期状态机更新。
**HTTP 探测门控**：Scanner 端口探测 + Fingerprinter 先得出 `service`，仅 `http/https` 的端口才触发 HTTP 探测（US-009）。

### 2.4 文件结构
```
atlas/
├── cmd/atlas/main.go                [NEW] 入口，装配所有组件
├── configs/atlas.yaml               [NEW] 默认配置
├── internal/
│   ├── config/config.go             [NEW]
│   ├── model/model.go               [NEW] Host/Port/Domain/Cert/Task/Vuln/...
│   ├── blacklist/blacklist.go       [NEW]
│   ├── store/
│   │   ├── pg.go                    [NEW] PostgreSQL 仓储
│   │   └── es.go                    [NEW] Elasticsearch 客户端
│   ├── queue/nats.go                [NEW] NATS 封装
│   ├── scheduler/scheduler.go       [NEW]
│   ├── scanner/
│   │   ├── probe.go                 [NEW] 多模式 Probe(ip,ports,mode,opts)
│   │   └── http.go                  [NEW] HTTP 探测（仅 http 服务）
│   ├── fingerprinter/
│   │   ├── engine.go                [NEW] 规则引擎
│   │   └── wappa.go                 [NEW] wappalyzergo 集成
│   ├── vuln/
│   │   ├── engine.go                [NEW] 模板执行 Check()
│   │   ├── template.go              [NEW] nuclei 风格模板解析
│   │   └── aider.go                 [NEW] 带外验证
│   ├── ratelimit/ratelimit.go       [NEW] 令牌桶
│   ├── audit/audit.go               [NEW]
│   └── api/
│       ├── router.go                [NEW] Gin 路由
│       ├── handler_*.go             [NEW] 各资源 handler
│       └── web/                     [NEW] Vue 3 构建产物（build 后放入）
├── migrations/                      [NEW] golang-migrate SQL
├── templates/                       [NEW] 内置 nuclei 风格检测模板（含 Kunpeng 迁移）
├── fingerprints/                    [NEW] 指纹规则文件（yaml/json）
├── Dockerfile                       [NEW] 多阶段构建单二进制
├── docker-compose.yml               [NEW] atlas + nats + postgres + elasticsearch
└── go.mod                           [NEW]
```

---

## 3. 数据模型

### 3.1 PostgreSQL Schema（DDL）
```sql
-- 主机资产
CREATE TABLE hosts (
  ip          TEXT PRIMARY KEY,
  asn         INT,
  org         TEXT,
  geo         JSONB,
  os          TEXT,
  open_ports  INTEGER[],
  first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_hosts_last_seen ON hosts(last_seen);

-- 端口/服务
CREATE TABLE ports (
  ip        TEXT NOT NULL,
  port      INT  NOT NULL,
  proto     TEXT NOT NULL DEFAULT 'tcp',
  service   TEXT,
  version   TEXT,
  banner    TEXT,
  cert      JSONB,
  title     TEXT,
  webinfo   JSONB,
  first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (ip, port, proto)
);
CREATE INDEX idx_ports_service ON ports(service);

-- 域名（二期被动源用，先建表留空）
CREATE TABLE domains (
  name     TEXT PRIMARY KEY,
  registrable_domain TEXT,
  resolved_ips TEXT[],
  cname    TEXT[],
  whois    JSONB,
  cert     JSONB,
  first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 资产历史快照
CREATE TABLE asset_history (
  id BIGSERIAL PRIMARY KEY,
  entity_type TEXT,
  entity_key  TEXT,
  del_time    TIMESTAMPTZ,
  type        TEXT,   -- update
  snapshot    JSONB
);
CREATE INDEX idx_asset_hist_key ON asset_history(entity_type, entity_key);

-- 黑名单
CREATE TABLE blacklist (
  id BIGSERIAL PRIMARY KEY,
  type TEXT NOT NULL,        -- ip | cidr | domain
  value TEXT NOT NULL,
  operator TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (type, value)
);

-- 任务
CREATE TABLE tasks (
  id UUID PRIMARY KEY,
  kind TEXT NOT NULL,        -- scan | vuln
  scope JSONB NOT NULL,
  schedule JSONB,
  rate_limit JSONB,
  status INT NOT NULL DEFAULT 0,   -- 0 pending 1 running 2 done
  progress JSONB,            -- {total:int, done:int}
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_tasks_status ON tasks(status);

-- 任务子项（断点续扫）
CREATE TABLE task_items (
  id BIGSERIAL PRIMARY KEY,
  task_id UUID NOT NULL,
  target TEXT NOT NULL,
  status INT NOT NULL DEFAULT 0,   -- 0 pending 1 done 2 filtered
  result JSONB,
  UNIQUE (task_id, target)
);
CREATE INDEX idx_items_task_status ON task_items(task_id, status);

-- 漏洞
CREATE TABLE vulns (
  id BIGSERIAL PRIMARY KEY,
  asset_ref TEXT NOT NULL,
  kpid TEXT,
  cve TEXT,
  name TEXT,
  level INT,
  type TEXT,
  proof TEXT,
  status TEXT NOT NULL DEFAULT 'open',  -- open|fixed|recur
  first_found TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_verified TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (asset_ref, kpid)
);
CREATE INDEX idx_vulns_status ON vulns(status);

-- 审计（可开关）
CREATE TABLE audit_logs (
  id BIGSERIAL PRIMARY KEY,
  operator TEXT,
  time TIMESTAMPTZ NOT NULL DEFAULT now(),
  target TEXT,
  task_id UUID,
  action TEXT
);

-- 检测模板（nuclei 风格）
CREATE TABLE templates (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,    -- yaml | go
  content TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true
);

-- 配置
CREATE TABLE config (
  key TEXT PRIMARY KEY,
  value JSONB NOT NULL
);
```

### 3.2 Elasticsearch 索引映射（assets）
```json
{
  "mappings": {
    "properties": {
      "ip":     { "type": "ip" },
      "port":   { "type": "integer" },
      "proto":  { "type": "keyword" },
      "service":{ "type": "keyword" },
      "version":{ "type": "keyword" },
      "banner": { "type": "text" },
      "title":  { "type": "text" },
      "tag":    { "type": "keyword" },
      "hostname":{ "type": "keyword" },
      "server": { "type": "keyword" },
      "time":   { "type": "date" }
    }
  }
}
```
写入策略：每次 `ports` 行 upsert 时同步写/更新 ES 文档（按 `ip:port` 为 `_id`）；检索由 API 转 ES DSL。

### 3.3 实体定义（Go 结构体摘要）
```go
type Host struct {
    IP, ASN int; Org string; Geo map[string]any; OS string
    OpenPorts []int; FirstSeen, LastSeen time.Time
}
type Port struct {
    IP, Port int; Proto, Service, Version, Banner, Title string
    Cert, WebInfo map[string]any; FirstSeen, LastSeen time.Time
}
type Task struct {
    ID uuid.UUID; Kind string; Scope, Schedule, RateLimit map[string]any
    Status int; Progress Progress; CreatedAt time.Time
}
type Vuln struct {
    AssetRef, KPID, CVE, Name, Type, Proof string
    Level int; Status string // open|fixed|recur
    FirstFound, LastVerified time.Time
}
type BlacklistItem struct { Type, Value, Operator string; CreatedAt time.Time }
```

### 3.4 关系
- `task_items.task_id → tasks.id`（1:N）
- `vulns.asset_ref → ports(ip:port)`（逻辑引用，非外键，避免跨表锁）
- `asset_history` 为 `hosts`/`ports` 的变更快照副本
- 其余为独立实体（blacklist / audit_logs / templates / config）

### 3.5 迁移计划
- 使用 `golang-migrate`，首次启动自动执行 `migrations/*.up.sql`。
- 向后兼容：本期为首版，无历史数据；后续加字段走新迁移文件，禁止 `ALTER` 删除列。
- 回滚：保留 `.down.sql`；生产回滚仅用于紧急，数据以快照为准。

---

## 4. API 设计

### 4.1 端点表
| 方法 | 路径 | 说明 | 鉴权 | 请求 | 响应 |
|------|------|------|------|------|------|
| GET | `/api/v1/blacklist` | 黑名单列表 | 管理员 | — | 列表 |
| POST | `/api/v1/blacklist` | 新增黑名单项 | 管理员 | {type,value} | 创建项 |
| POST | `/api/v1/blacklist/import` | 批量导入 | 管理员 | {items:[{type,value}]} | {success, fail, errors} |
| DELETE | `/api/v1/blacklist/:id` | 删除项 | 管理员 | — | 成功 |
| POST | `/api/v1/tasks` | 创建扫描/漏洞任务 | 管理员 | TaskCreate | {task_id} |
| GET | `/api/v1/tasks` | 任务列表 | 管理员 | — | 列表 |
| GET | `/api/v1/tasks/:id` | 任务详情/进度 | 管理员 | — | 任务 |
| POST | `/api/v1/tasks/:id/cancel` | 取消任务 | 管理员 | — | 成功 |
| GET | `/api/v1/assets/search?q=` | 资产检索（ES） | 管理员 | q=key:value | 分页结果 |
| GET | `/api/v1/assets/:ip/:port` | 资产详情 | 管理员 | — | 端口详情 |
| GET | `/api/v1/vulns` | 漏洞列表 | 管理员 | 过滤参数 | 列表 |
| GET | `/api/v1/vulns/:id` | 漏洞详情 | 管理员 | — | 详情 |
| GET | `/api/v1/vulns/export?fmt=csv` | 导出报表 | 管理员 | — | 文件 |
| GET | `/api/v1/audit?task_id=` | 审计查询 | 管理员 | — | 列表（开关关时为空） |
| GET/PUT | `/api/v1/config` | 读取/更新配置（限速、审计开关、扫描默认） | 管理员 | Config | 配置 |
| POST | `/api/v1/auth/login` | 管理员登录 | 公开 | {user,pass} | token |

### 4.2 关键请求/响应示例
**创建扫描任务** `POST /api/v1/tasks`
```json
{
  "kind": "scan",
  "scope": { "targets": ["1.2.3.0/24"], "port_range": "top1000",
             "scan_mode": "connect", "blacklist_ref": true },
  "schedule": { "type": "once" },
  "rate_limit": { "max_concurrency": 500, "per_target_rps": 10 }
}
```
响应：`{ "task_id": "uuid", "status": "pending" }`

**资产检索** `GET /api/v1/assets/search?q=port:443%20server:nginx`
响应：`{ "total": 123, "page": 1, "items": [ { "ip":"...", "port":443, "service":"https", "title":"...", ... } ] }`

### 4.3 错误响应
统一结构：`{ "error": { "code": "BAD_REQUEST", "message": "..." } }`
| 场景 | HTTP | code |
|------|------|------|
| 参数校验失败 | 400 | BAD_REQUEST |
| 目标命中黑名单（创建即拦截） | 400 | TARGET_BLOCKED |
| 未认证/无权限 | 401/403 | UNAUTHORIZED/FORBIDDEN |
| 模板执行超时 | 504 | UPSTREAM_TIMEOUT |
| 内部错误 | 500 | INTERNAL |

### 4.4 破坏性变更
本期为首版，无既有消费者，无破坏性变更。

---

## 5. 业务逻辑

### 5.1 核心算法
**端口探测** `Probe(ip, ports, mode, opts)`：
1. 按 `mode` 选择实现：connect 用 `net.DialTimeout`；syn/udp/fin/null/xmas/ack 用 `gopacket`+`libpcap`（Linux），不支持环境回退 connect。
2. 端口来源：扫描任务 `port_range`（top1000/列表/区间/1..65535）。
3. 开放端口收集 banner，写入 `ports`。

**HTTP 探测门控**：仅当 `ports.service ∈ {http,https}` 时发 HTTP 请求；常见 Web 端口未识别服务时按补充策略尝试（默认开、可关）。

**指纹识别**：对 banner/header/body/title 跑规则引擎 + wappalyzergo；输出 service/component/version/tag 写回 `ports`。

**漏洞检测** `Check(task)`：加载模板（yaml/go）→ 按 `request` 发请求 → 按 `matchers` 判定 → 返回结构化结果；aider 用于无回显场景（DNS/HTTP 带外）。

**限速令牌桶**：`internal/ratelimit` 用 `golang.org/x/time/rate`；全局桶 + 每目标桶；超额请求排队（场景：Worker 取任务前先 `Wait()`）。

**断点续扫**：任务分解为 `task_items`（status 0）；Worker 仅取 status=0 项；进程重启后 Scheduler 从 `task_items` 继续派发未完成任务，不重头。

### 5.2 校验规则
- 黑名单项 `type ∈ {ip,cidr,domain}`，CIDR 须经 `net.ParseCIDR` 校验。
- 任务 `port_range` 须为 top1000/列表/区间/全端口之一；`scan_mode` 须为枚举值。
- 创建任务时若 `scope.targets` 全部命中黑名单且无其他目标 → 返回 `TARGET_BLOCKED`。
- 速率上限须为正整数，且不超过全局硬上限（由部署者配置）。

### 5.3 状态机
```
Task:   pending ──▶ running ──▶ done
                    │            ▲
                    └── cancel ──┘
Item:   pending ──▶ done
               └──▶ filtered (命中黑名单)
Vuln:   open ──▶ fixed ──▶ open(recur)
```
转换守卫：仅 Scheduler/Worker 可改状态；`done` 不可回退为 `pending`（除 cancel 重开新任务）。

### 5.4 边界情况
- 目标全在黑名单内：任务立即 `done`，进度 total=0。
- 探测中途进程崩溃：依靠 `task_items` 断点，重启续扫。
- raw socket 不可用（Windows）：自动回退 connect 并告警。
- ES 暂不可达：资产仍写 PG，ES 写入失败进重试队列（PG 标记 `es_pending`），恢复后补索引。
- 审计开关关闭：所有 `audit_logs` 写入被短路。

---

## 6. 错误处理

### 6.1 错误分类
| code | HTTP | 条件 |
|------|------|------|
| BAD_REQUEST | 400 | 参数/格式错误 |
| TARGET_BLOCKED | 400 | 目标命中黑名单 |
| UNAUTHORIZED | 401 | 未登录 |
| FORBIDDEN | 403 | 非管理员 |
| UPSTREAM_TIMEOUT | 504 | 探测/模板超时 |
| INTERNAL | 500 | 未预期错误 |

### 6.2 重试策略
- 网络探测失败：按 `scan_mode` 重试 ≤2 次（指数退避），仍失败标记端口关闭/未知。
- ES 写入失败：进本地重试队列，上限 5 次，仍失败记 PG `es_pending` 待补。
- NATS 发布失败：本地落盘 `task_items` 已持久化，发布可重放。

### 6.3 失败模式
- PG 不可用：服务启动失败（强依赖）。
- NATS 不可用：单实例仍可运行（进程内派发）；多实例横向失效，降级为单实例容量。
- ES 不可用：见 5.4，资产不丢，检索降级为 PG 兜底（LIKE/JSONB 查询）。

---

## 7. 安全

### 7.1 认证授权
- 单管理员模型：登录后发 JWT（短期）；`config` 表存管理员口令哈希（env 初始化）。
- 所有 `/api/v1/*` 除 login 外须带有效 token。

### 7.2 输入校验
- 所有外部输入经结构体校验（边界、类型、枚举）。
- 模板 `content` 执行前静态检查：禁止写操作/利用载荷（如含危险函数调用则拒绝加载）；模板在受限上下文中运行。
- CIDR/域名规范化后再存，防注入。

### 7.3 数据保护
- 审计日志受开关控制，启用时记录操作人/时间/目标/动作。
- **法律警示强制展示**：前端登录后首页 + API 根路径都返回合规提示横幅；部署文档显著位置提示未经授权扫描第三方资产可能违法。
- 敏感字段（如 banner 中可能含凭证）按原始存储，不额外脱敏（运营自管）。

---

## 8. 性能

### 8.1 预期负载
- 单实例：全端口探测吞吐受令牌桶约束，建议值约 ≥1,000 IP/天（按默认限速）。
- 百万级：通过 N 实例线性扩展（实例数 ≈ 目标IP数 / 单实例日处理IP数）。

### 8.2 优化策略
- 端口探测用 `ants` 协程池 + 非阻塞 IO；连接复用（`fasthttp` 连接池）。
- ES 批量 bulk 写入（攒批 ≤ 200 条或 1s 刷）。
- 任务子项分页取，避免大事务。

### 8.3 数据库考量
- `ports(ip,port,proto)` 主键，命中索引检索；`service`、`status` 建索引。
- ES 检索走 `ip`/`keyword` 精确 + `text` 全文；中文 banner 走 `regex`、其余走 `match`。
- 防 N+1：资产详情一次 `JOIN`/聚合取全端口。

---

## 9. 测试策略

### 9.1 单元测试
- `scanner/probe`：各 mode mock 网络，验证开放/关闭判定与回退。
- `blacklist`：IP/CIDR/域名命中逻辑。
- `ratelimit`：令牌桶速率与排队。
- `fingerprinter`：规则与 wappalyzergo 输出。
- `vuln/template`：matchers 命中/未命中。

### 9.2 集成测试
- 端到端：创建扫描任务 → 消费 → PG/ES 写入 → 检索返回。
- NATS：多实例消费均衡。
- API：鉴权、黑名单拦截、审计开关。

### 9.3 边界测试
- 全黑名单目标任务；进程重启续扫；ES 不可达降级；raw socket 回退。

### 9.4 验收映射
| US/FR | 测试类型 | 说明 |
|-------|----------|------|
| US-001/002 FR-1 | 集成 | 黑名单录入/导入 |
| US-003 FR-2 | 单元+集成 | 扫描前过滤命中黑名单 |
| US-004 FR-3 | 单元 | 限速令牌桶 |
| US-005 FR-4 | 集成 | 审计开关 |
| US-008 FR-6 | 单元 | 多模式 + 端口范围 |
| US-009 FR-7 | 集成 | 仅 http 服务才 HTTP 探测 |
| US-010 FR-8 | 单元 | 指纹规则 + wappalyzergo |
| US-011 FR-9 | 集成 | PG upsert |
| US-012 FR-10 | 集成 | ES 检索语法 |
| US-014/015 FR-12/13 | 单元 | 模板 Check + matchers |
| US-016 FR-14 | 集成 | aider 带外 |
| US-020 FR-18 | 集成 | 断点续扫 + 多实例 |
| US-021 FR-19 | 集成 | 历史快照对比 |

---

## 10. 实现计划

### 10.1 阶段
**阶段一（资产测绘主线，优先）**：基础设施（config/store/queue/ratelimit/audit）+ 黑名单（US-001~003）+ 任务与调度（US-006,007,020）+ 扫描探测（US-008,009）+ 指纹（US-010）+ 资产建模与检索（US-011,012,013）+ 历史快照（US-021）+ API/前端 + Docker（FR-20）。
**阶段二（漏洞引擎主线）**：VulnEngine 框架（US-014）+ 模板体系（US-015）+ aider（US-016）+ 漏洞任务（US-017）+ 漏洞存储/生命周期（US-018）+ 漏洞展示/报表（US-019）+ Kunpeng → 模板迁移。

### 10.2 Issue 映射
| Issue | SPEC 章节 | 优先级 | 依赖 |
|-------|-----------|--------|------|
| #1 基础设施与存储 | 2.2,3.1,3.5 | high | — |
| #2 黑名单 | 2.2,4.1,5.2 | high | #1 |
| #3 调度+任务 | 2.3,4.1,5.1 | high | #1 |
| #4 端口/HTTP 探测 | 2.2,5.1 | high | #3 |
| #5 指纹引擎 | 2.2,5.1 | high | #4 |
| #6 资产建模+检索 | 3.1,3.2,4.1 | high | #4,#5 |
| #7 历史快照 | 3.1,5.4 | mid | #6 |
| #8 API+前端 | 4.1,7 | high | #2~#6 |
| #9 Docker 交付 | 2.4,FR-20 | mid | #8 |
| #10 漏洞引擎框架 | 2.2,5.1 | high | #8 |
| #11 模板+aider | 4.1,5.1 | high | #10 |
| #12 漏洞存储/展示 | 3.1,4.1 | high | #11 |
| #13 Kunpeng 迁移 | 3.3 | mid | #11 |

### 10.3 增量交付
- 阶段一即可独立交付可用资产测绘产品；阶段二在其上叠加漏洞检测。
- 每 Issue 完成后经 `/review-it` 再合入；通过 feature 分支隔离。

---

## 11. 开放问题 & 风险

### 11.1 未决问题
- 管理员口令初始化方式（首启随机生成 vs env 固定），待定。

### 11.2 技术风险
| 风险 | 影响 | 缓解 |
|------|------|------|
| raw socket 在 Windows 不支持 SYN/UDP | 部分模式失效 | 自动回退 connect + 告警 |
| ES 集群资源占用 | 部署变重 | docker-compose 默认单节点，生产可独立扩容 |
| wappalyzergo 签名覆盖不足 | 指纹漏报 | 自研规则热加载补充 |
| 单实例令牌桶限速跨实例不全局 | 多实例总体超速 | 限速按实例配置，依赖部署者按实例数反推总速 |

### 11.3 假设
- 限速默认值（并发 500 / 每目标 10 rps）为 [假设]，以上线压测校准（OQ-3）。
- 多实例间无全局限速协调，总速 ≈ 单实例速 × 实例数（OQ-3）。
- nuclei 风格模板的 `matchers` 类型集首版支持 word/regex/binary/status/size，后续扩展。
- ASN/Org/Geo 字段 MVP 留空，二期经 WHOIS/ASN/RDAP 补全（OQ-5）。
