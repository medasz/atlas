# Atlas — 互联网资产与漏洞测绘平台

Atlas 是一个**单机二进制**的互联网资产测绘与漏洞检测平台：对 IP / 域名进行端口探测、服务/组件指纹识别，并将结果统一建模后持久化到 PostgreSQL、同步索引到 Elasticsearch，提供类 FOFA 的 Dork 检索语法与 Web 控制台。漏洞检测引擎仅做**验证**（发送探测请求判断是否存在），**不做任何利用**。

> ⚠️ 合规要求：仅对**你拥有授权**的资产进行扫描。控制台强制展示法律横幅，使用前请确认已取得合法授权。

---

## 特性

- **单二进制、非 cgo**：`CGO_ENABLED=0` 编译，无外部动态库依赖，便于分发与容器化。
- **多实例水平扩展**：多个 Atlas 实例共享同一套 NATS / PostgreSQL / Elasticsearch；任务经 NATS 队列在实例间分发（队列组负载均衡），进程内调用走直接函数调用。
- **统一资产模型**：主机（Host）、端口/服务（Port）、域名（Domain）三类资产，按主键冲突更新并保留 `first_seen` / `last_seen`。
- **PG + ES 双存储**：PostgreSQL 为权威持久层；Elasticsearch 提供全文/term 检索。ES 不可达时自动降级为 PostgreSQL 兜底，索引失败的文档进入内存待补队列并置 `es_pending`，后台每 30s 重试。
- **类 FOFA Dork 检索**：支持 `field==/!=/*=/=` 运算符、`&&` / `||` 逻辑、`()` 分组优先级，覆盖 ip/port/protocol/server/banner/title/os/org/asn/host/domain/app/product/body/header/country/region/city/cert/is_ipv6 等字段，裸词走全字段全文检索。
- **指纹识别**：集成 [wappalyzergo](https://github.com/projectdiscovery/wappalyzergo) 社区库 + 自研 YAML 规则热加载（`POST /api/fingerprint/reload` 或 `SIGHUP` 信号）。
- **漏洞引擎**：自研 Go 实现的 nuclei 风格 YAML 模板检测（仅验证不利用），模板可经 API 热添加并持久化。

---

## 架构概览

```
                ┌────────────┐   ┌────────────┐
 扫描目标 ─────▶ │ Atlas 实例 │   │ Atlas 实例 │  (水平扩展，共享后端)
                └─────┬──────┘   └─────┬──────┘
                      │   NATS 任务队列（跨实例分发）
                      └─────────┬──────────┘
                      ┌─────────┴──────────┐
                  ┌───┴────┐          ┌────┴─────┐
                  │Postgres│◀────────▶│Elasticse-│
                  │  (权威)│  同步索引  │  arch    │
                  └────────┘          └──────────┘
```

- **PostgreSQL**：资产 / 任务 / 漏洞 / 审计 / 黑名单的持久化与兜底检索。
- **Elasticsearch**：资产检索（Dork 语法优先走 ES）。
- **NATS**：跨实例任务分发；连接失败则退化为单实例进程内执行。

---

## 技术栈

| 层 | 选型 |
|----|------|
| 语言 | Go 1.22+（非 cgo） |
| HTTP | gin |
| 持久化 | PostgreSQL（pgx/v5 `pgxpool`） |
| 检索 | Elasticsearch（标准库 `net/http` 客户端，无额外依赖） |
| 队列 | NATS（`nats.go`） |
| 指纹 | wappalyzergo + 自研 YAML 规则 |
| 前端 | Vue 3 + Element Plus + Vite |

---

## 目录结构

```
atlas/
├── cmd/atlas/main.go      # 入口：装配存储/队列/扫描/漏洞/服务，信号处理
├── configs/               # atlas.yaml、fingerprint-rules.yaml、templates/
├── migrations/            # 幂等 SQL 迁移（golang-migrate 风格，按 schema_migrations 去重）
├── internal/
│   ├── config/            # 配置加载（YAML + 环境变量覆盖）
│   ├── store/             # PG 仓储 + ES 同步 + Dork 解析器（query.go）
│   ├── scan/              # 端口/HTTP 探测、IPv6 识别、域名资产登记
│   ├── fingerprint/       # 指纹识别服务（社区库 + 热加载规则）
│   ├── vuln/              # 漏洞检测引擎（nuclei 风格模板）
│   ├── task/              # 任务调度与执行
│   ├── server/            # HTTP API 与路由
│   ├── queue/ blacklist/ audit/ ratelimit/ model/   # 支撑模块
├── web/                   # Vue 3 前端（npm run build → dist/）
├── docker-compose.yml     # 一键起 PG + NATS + ES + 两个 Atlas 实例
└── Dockerfile             # 多阶段构建（gobuild + webbuild + 运行镜像）
```

---

## 快速开始

### 方式一：Docker Compose（推荐）

```bash
docker-compose up -d --build
```

启动后包含：
- PostgreSQL（:5432）、NATS（:4222）、Elasticsearch（:9200）
- 两个 Atlas 实例（:8080 与 :8081，演示横向扩展），均托管前端 SPA

访问 http://localhost:8080 打开控制台，默认管理员口令见配置（`Auth.Password`，默认为 `admin`，**生产务必修改**）。

### 方式二：本地构建运行

前置：Go 1.22+、PostgreSQL、可选 NATS/Elasticsearch。

```bash
# 后端
go build -o atlas ./cmd/atlas
./atlas -config configs/atlas.yaml -migrations migrations \
        -rules configs/fingerprint-rules.yaml -templates configs/templates \
        -webdir web/dist

# 前端（可选，独立开发）
cd web && npm install && npm run dev
```

> 不指定 `-webdir` 时仅提供 API；指定后由同一进程托管前端 SPA。

---

## 配置

配置文件为 YAML（`configs/atlas.yaml`），关键连接项可被环境变量覆盖：

| 环境变量 | 对应配置 | 说明 |
|----------|----------|------|
| `ATLAS_PG_DSN` | `postgres.dsn` | PostgreSQL 连接串 |
| `ATLAS_NATS_URL` | `nats.url` | NATS 连接地址 |
| `ATLAS_ES_ADDR` | `elastic.addr` | Elasticsearch 地址 |
| `ATLAS_HTTP_ADDR` | `http.addr` | HTTP 监听地址 |

```yaml
http:
  addr: ":8080"
nats:
  url: "nats://127.0.0.1:4222"
postgres:
  dsn: "postgres://postgres:postgres@127.0.0.1:5432/atlas?sslmode=disable"
elastic:
  addr: "http://127.0.0.1:9200"
  index: "assets"
scan:
  default_mode: "connect"          # connect|syn|fin|null|xmas|udp|ack（当前实现为 connect/TCP）
  default_port_range: "top1000"    # top1000|list|range|1..65535
  max_concurrency: 500             # 单实例全局最大并发
  per_target_rps: 10               # 每目标请求速率限制
audit:
  enabled: true
auth:
  enabled: true
  password: "admin"                # 务必修改
  secret: "atlas-dev-secret-change-me"
```

### 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-config` | `configs/atlas.yaml` | 配置文件路径 |
| `-migrations` | `migrations` | SQL 迁移目录 |
| `-rules` | `configs/fingerprint-rules.yaml` | 指纹规则 YAML（不存在则仅用社区库） |
| `-templates` | `configs/templates` | 漏洞模板目录 |
| `-webdir` | 空 | 构建好的前端目录（设置后托管 SPA） |

---

## 数据模型与迁移

启动时自动执行 `migrations/*.up.sql`（幂等，已应用的迁移记录在 `schema_migrations` 表）。核心资产表：

- **hosts**：`ip`(PK)、`asn`、`org`、`geo`、`os`、`is_ipv6`、`open_ports`、`first_seen`、`last_seen`
- **ports**：`(ip, port, proto)`(PK)、`service`、`version`、`banner`、`title`、`host`、`is_ipv6`、`cert`、`webinfo`(JSON)、`first_seen`、`last_seen`
- **domains**：`name`(PK)、`registrable_domain`、`resolved_ips`、`cname`、`org`、`asn`、`is_ipv6`、`whois`(JSON)、`first_seen`、`last_seen`
- **vulns / tasks / task_items / blacklist / audit_logs / templates / schema_migrations**

---

## 资产检索（Dork 语法）

检索入口：`GET /api/assets?q=<Dork>&type=<host|port|domain|空>`。语法参考 FOFA：

- **运算符**：`=`（包含）、`==`（精确）、`!=`（排除）、`*=`（模糊/通配）
- **逻辑**：`&&`（与）、`||`（或）
- **分组**：`()` 改变优先级，如 `(a || b) && c`
- **裸词**：不带字段名时走全字段全文检索
- **作用域**：`type` 限定返回资产类型；`host`/`domain` 字段在不同作用域映射到不同列

### 支持字段

| 字段 | 含义 | 作用域映射 |
|------|------|-----------|
| `ip` | IP 地址 | hosts/ports 的 `ip` |
| `port` | 端口 | ports 的 `port` |
| `protocol` / `base_protocol` | 协议 | ports 的 `proto` |
| `server` | Server 头 | ports 的 `webinfo->server` |
| `banner` / `title` / `body` / `header` | 横幅/标题/正文/头 | 对应列 / 全字段 |
| `os` / `org` / `asn` | 操作系统/组织/ASN | hosts 对应列 |
| `host` | 到达端口所用的主机名（HTTP Host） | ports 的 `host` |
| `domain` | 域名 | port 作用域→`host` 列；domain 作用域→`domains.name` |
| `app` / `product` | 组件/产品 | ports 的 `webinfo->tech` |
| `country` / `region` / `city` | 地理位置 | hosts 的 `geo` |
| `cert` | 证书 | ports 的 `cert` |
| `is_ipv6` | 是否 IPv6 | `true`/`false`（term / 布尔比较） |

### 示例

```
ip="1.1.1.1"
ip="1.1.1.1" && port="443"
server="nginx" && (port="80" || port="443")
title="登录" && country="CN"
is_ipv6=true
domain="example.com"
host="admin.example.com" && port="443"
protocol="tcp" && banner*="SSH"
```

ES 不可用时自动回退到 PostgreSQL 的 `ILIKE` / 布尔 / 数值比较查询，资产不丢失。

---

## HTTP API 一览

除登录与健康检查外，所有 `/api` 路由需认证（会话 Cookie）。

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/login` | 登录（口令）→ 设置会话 Cookie |
| POST | `/api/logout` | 登出 |
| GET | `/api/assets?q=&type=` | Dork 检索资产（host/port/domain） |
| GET | `/api/hosts/:ip` | 主机资产详情 |
| GET | `/api/hosts/:ip/detail` | 主机详情：主机 + 全部端口（含指纹）+ 关联漏洞 |
| GET | `/api/tasks` | 任务列表 |
| POST | `/api/tasks` | 创建任务（`kind`: scan/vuln，含 scope/schedule/rate_limit） |
| GET | `/api/tasks/:id` | 任务详情（含子项） |
| POST | `/api/tasks/:id/resume` | 恢复/续跑任务 |
| GET | `/api/blacklist` | 黑名单列表 |
| POST | `/api/blacklist` | 新增黑名单（type: ip/cidr/domain） |
| DELETE | `/api/blacklist?type=&value=` | 删除黑名单 |
| GET | `/api/vulns?asset=` | 漏洞列表（按资产过滤） |
| GET | `/api/templates` | 漏洞模板列表 |
| POST | `/api/templates` | 新增漏洞模板（YAML） |
| POST | `/api/fingerprint/reload` | 热加载指纹规则 |

---

## 指纹识别

- 内置 [wappalyzergo](https://github.com/projectdiscovery/wappalyzergo) 社区库识别 Web 技术栈。
- 自研规则：在 `configs/fingerprint-rules.yaml` 中以 YAML 描述匹配（HTTP 头 / 正文 / 横幅 / 正则），支持 `banner` 维度匹配 TCP banner。
- 热加载：调用 `POST /api/fingerprint/reload` 或向进程发送 `SIGHUP` 信号，无需重启。

---

## 漏洞检测引擎

- nuclei 风格 YAML 模板，匹配 HTTP 响应头/正文判定是否存在漏洞；**仅验证、不利用**。
- 模板来源：启动加载 `configs/templates/` 目录 + 已持久化到数据库的模板（`POST /api/templates` 新增）。
- 检测结果写入 `vulns` 表，通过 `/api/vulns` 或主机详情的「关联漏洞」查看。

---

## 运维与信号处理

- `SIGINT` / `SIGTERM`：优雅退出。
- `SIGHUP`：重新加载指纹规则（等价于 `POST /api/fingerprint/reload`）。
- Elasticsearch 同步降级：ES 不可达时不阻断写入；失败文档进入内存队列，后台每 30s 重试并清除 `es_pending` 标记。

---

## 开发

```bash
# 测试（检索解析器等）
go test ./internal/store/...

# 静态检查
go vet ./...

# 前端
cd web && npm install && npm run build
```

法律与安全：本工具仅用于已授权资产的资产梳理与漏洞验证，严禁用于未授权目标。
#   a t l a s  
 