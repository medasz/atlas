# 配置界面网卡设置项改为下拉选择 — 设计文档

- 日期：2026-07-28
- 状态：已批准设计，待实现

## 1. 背景与目标

当前系统设置界面的「抓包网卡 (RawIface)」为手动文本输入框（`web/src/views/Settings.vue`）。用户需自行输入网卡名（如 `eth0`），易拼写错误且不便。本功能将该文本框改为**下拉选择框**，下拉选项由后端自动扫描并列出**当前提供 API 的 atlas 后端实例主机**的所有网络接口，用户直接选择即可，提升配置准确性与易用性。

`raw_iface` 经现有 `GET/PUT /api/config` 接口持久化到数据库（scan 段），运行时通过 `Scanner.SetScanConfig` 热更新，对新建任务即时生效——既有机制不变。

## 2. 范围

- 后端新增一个只读端点，返回当前后端实例主机的网络接口列表。
- 前端将该设置项由文本输入改为下拉选择，选项来源于上述端点。
- 保留「自动（默认）」空选项（空值 = 自动选出口网卡）。

## 3. 非目标

- **不**实现跨实例（atlas/atlas2）网卡合并；仅返回处理请求的后端实例主机网卡（已与用户确认）。
- **不**新增手动文本输入回退框；仅在接口列表获取失败时静默降级为仅「自动」选项。
- **不**修改 scan/raw 抓包实现或配置存储结构。

## 4. 后端设计

### 4.1 端点

- 方法/路径：`GET /api/config/interfaces`
- 鉴权：与 `/api/config` 同组，受 `authRequired` 保护（管理员会话）
- 依赖：仅标准库 `net`

### 4.2 返回结构

```json
[
  { "name": "lo",   "addrs": ["127.0.0.1/8", "::1/128"] },
  { "name": "eth0", "addrs": ["192.168.1.10/24"] }
]
```

Go 结构：

```go
type ifaceInfo struct {
    Name  string   `json:"name"`
    Addrs []string `json:"addrs"`
}
```

### 4.3 实现要点

- 调用 `net.Interfaces()`；对返回错误返回 500。
- 遍历每个 `net.Interface`，调用其 `Addrs()` 收集地址字符串（CIDR 形式，如 `192.168.1.10/24`）。
- **不排除任何接口**（含 `lo`、容器 overlay 网卡等），完整列出由用户自行判断选择。
- 顺序保持 `net.Interfaces()` 返回顺序。

## 5. 前端设计

### 5.1 api 调用层

新增 `getInterfaces()`：请求 `GET /api/config/interfaces`，返回 `ifaceInfo[]`。

### 5.2 Settings.vue

- 状态：新增 `interfaces`（ref 数组），`form.rawIface` 仍绑定 `<select>` 的 `v-model`。
- 加载时机：`onMounted` 在 `load()` 之后（或并行）调用 `loadInterfaces()` 填充 `interfaces`。
- 模板：将 `rawIface` 的 `<input type="text">` 替换为 `<select>`：
  - 固定选项：`<option value="">自动（默认，留空）</option>`
  - 动态选项：`v-for="i in interfaces" :key="i.name" :value="i.name"`，显示文本为 `i.name`，若 `i.addrs` 非空则附加 ` (i.addrs[0])`。
- hint 文案：「raw 模式抓包所用网卡；当前后端实例主机可用接口如下（通常选非 lo 的出口网卡）」。

### 5.3 错误降级

`loadInterfaces` 失败时：静默处理（或置 `notice`），`interfaces` 保持为空 → 下拉仅剩「自动（默认）」选项，页面不卡死，`rawIface` 仍可留空保存。不与配置加载错误（`error`）混淆。

## 6. 数据流

```
前端加载
  └─ GET /api/config/interfaces
       └─ 后端 net.Interfaces()
            └─ []ifaceInfo
                 └─ 前端填充下拉选项

用户选择/留空
  └─ 随 PUT /api/config 的 scan.raw_iface 提交
       └─ 现有持久化（DB）+ SetScanConfig 热更新（不变）
```

## 7. 测试

### 7.1 后端单测

使用 `net/http/httptest` 启动 `Server.Engine()`（或构造带 `Deps` 的 `Server`），请求 `/api/config/interfaces`（绕过鉴权或用测试登录会话），断言：

- 状态码 200，Content-Type `application/json`
- 返回为数组，且至少包含一个本机必有的接口（如 `lo`），或长度 > 0 且元素含 `name` 字段

注：`net.Interfaces` 不可 mock，测试依赖运行环境真实网卡；至少验证非错误响应与结构正确。

### 7.2 前端

手动验证：设置页下拉渲染接口列表、选择后保存、重新加载回填正确。若 web 项目含 vitest，可补一个下拉渲染/选项单元用例（可选）。

## 8. 文件改动清单

- `internal/server/config.go`：新增 `listInterfaces` 处理器，并在 `registerConfig` 注册 `GET /interfaces`
- `web/src/api`（对应模块，如 `api.ts`/`index.ts`）：新增 `getInterfaces()`
- `web/src/views/Settings.vue`：`rawIface` 输入改 `select` + 加载逻辑 + 错误降级

## 9. 风险与缓解

- **多实例差异**：不同实例网卡可能不同，下拉仅反映当前请求落到的实例。已与用户确认采用「当前后端实例主机」语义，属预期，非缺陷。
- **容器环境网卡名**：容器内网卡名（如 `eth0`、`docker0`、`br-xxx`）可能与宿主机不同；用户需选容器内实际抓包网卡，列表已如实反映容器内接口，符合预期。
- **鉴权暴露面**：端点受 `authRequired` 保护，与 `/config` 一致，无新增未授权暴露。

## 10. 验收标准

1. 设置页「抓包网卡」为下拉框，选项含「自动（默认）」+ 后端实例全部网卡（带 IP 提示）。
2. 选择某网卡保存后，重新进入设置页正确回填。
3. 接口列表获取失败时页面仍可用（仅「自动」选项）。
4. 现有配置读写与运行时热更新行为不受影响。
