# 端口开放状态展示（主列表状态徽标）设计方案

- 日期：2026-07-31
- 状态：设计中（待评审）
- 范围：本方案是「给所有扫描出来的端口加上状态」的最小实现；此处「状态」指**端口开放状态**（open / closed / filtered / open|filtered / unfiltered / timeout），非业务状态。

## 1. 背景

`model.Asset` 已有 `State` 字段（`state`），用于存储 TCP 扫描开放状态。现状问题：

1. **前端未展示端口状态**：主列表端口列仅显示端口号 / 开放端口列表，主机详情端口子表也未显示状态。
2. **域名扫描端口缺 State**：`scanDomain` 生成的 http 端口只写了 `Service:"http"`，未写 `State`，导致这类端口状态缺失（IP 扫描路径 `connect`/`persistResult` 已正确写入 `State`）。

用户决策：端口开放状态展示在**主列表端口列**（彩色徽标）；主机详情弹窗只保留该端口的**漏洞**与**深度指纹**（version/title/server/tech），端口状态不进入详情。

## 2. 目标

- 所有扫描出来的端口都带有开放状态（补齐域名扫描端口）。
- 主列表端口列以彩色徽标呈现每端口开放状态，便于一眼筛查。
- 主机详情弹窗维持「漏洞 + 逐端口深度指纹」的下钻定位，不含状态列。

## 3. 设计

### 3.1 后端：补齐域名扫描端口的 State

`internal/scan/scan.go` 的 `scanDomain` 中构造 http `portAsset` 时增加：

```go
State: string(tcpscan.Open),
```

探测成功即端口开放，`tcpscan.Open` 值为 `"open"`，与 IP 扫描路径语义一致。IP 扫描路径已有 `State`，无需改动。

### 3.2 后端：聚合结果携带 port_states

`internal/esasset/esasset.go` 的 `SearchAssets` 聚合（IP 桶）循环里，遍历 `top_docs` 的 `_source` 时，除收集 `portsMap` 外，顺便收集 `portStates map[string]string`（key=`strconv.Itoa(port)`，value=`state`）：

```go
if p, ok := src["port"].(float64); ok && p > 0 {
    portsMap[int(p)] = true
    if st, ok := src["state"].(string); ok && st != "" {
        portStates[strconv.Itoa(int(p))] = st
    }
}
```

并在合并项 `item` 写入：

```go
item["port_states"] = portStates
```

- `port_states` 仅出现在**搜索返回**的聚合项中，不写 ES，**无需改 `assetMapping`**。
- 非聚合模式返回 ES 原始文档，端口文档自带 `state`，前端直接使用 `row.state`。

### 3.3 前端：主列表端口列渲染状态徽标

`web/src/views/Assets.vue` 主列表「端口」列（当前为 `row.open_ports.join(', ')` 或 `row.port`）改为渲染状态徽标：

- 单端口文档（`row.port` 存在）：徽标显示端口号，按 `row.state` 着色。
- 聚合主机（`row.open_ports` 列表）：对每个端口，用 `row.port_states[端口]` 取状态着色，渲染徽标列表。
- 域名行：显示 `-`。

状态 → 配色：

- `open` → 绿（开放）
- `filtered` / `open|filtered` / `unfiltered` → 琥珀（不确定）
- `closed` / `timeout` → 红 / 灰（关闭 / 超时）
- 空 → 中性灰（未知）

### 3.4 前端：主机详情弹窗（不添加状态列）

详情弹窗的端口子表维持现有「深度指纹」列（port / proto / service / version / title / 服务(Web) / 技术栈 / IPv6），**不新增状态列**；漏洞子表保持不变。端口状态仅在主列表呈现，保持「主列表总览、详情下钻」的边界。

## 4. 数据形状

聚合主机项新增字段：

```json
{
  "ip": "1.1.1.1",
  "open_ports": [443, 22],
  "port_states": {"443": "open", "22": "open"},
  "services": ["https", "ssh"],
  "os": "Linux"
}
```

## 5. 取舍

- 以独立 `port_states` map 附加到聚合项，而非把 `open_ports` 改为对象数组——后者会破坏依赖 `open_ports` 为 int 列表的代码与测试。
- 状态不写 ES、详情不重复展示，避免详情冗余，明确总览/下钻分工。

## 6. 测试

- `internal/esasset/esasset_test.go` 的 `TestSearchAssets_Aggregated` 增加断言：合并项含 `port_states`，且 `1.1.1.1` 对应端口的状态与 fixture 的 `_source` 实际取值一致（如 `443` → `"open"`、`22` → `"open"`）。

## 7. 实现顺序

1. `scan.go` `scanDomain` 补 `State: string(tcpscan.Open)`。
2. `esasset.go` 聚合循环补 `portStates` 收集并写入 `item["port_states"]`。
3. `Assets.vue` 主列表端口列渲染状态徽标（新增状态配色与徽标样式）。
4. 补充测试断言 + `go build` / `go test` / `go vet` 验证。

## 8. 风险

- 低：纯展示增强，不改 ES mapping、不改存储契约；前端仅改主列表端口列渲染逻辑与样式。
