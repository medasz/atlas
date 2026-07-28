# 配置界面网卡设置项改为下拉选择 — 实现计划

- 日期：2026-07-28
- 关联设计：`docs/superpowers/specs/2026-07-28-config-iface-select-design.md`（已批准）
- 目标：将设置页「抓包网卡 (RawIface)」文本框改为下拉框，选项由后端 `GET /api/config/interfaces` 返回当前后端实例主机所有网络接口。

## 实现概览

| 层 | 文件 | 改动 |
|----|------|------|
| 后端 | `internal/server/config.go` | 新增 `listInterfaces` 处理器 + 在 `registerConfig` 注册 `GET /interfaces` |
| 后端测试 | `internal/server/config_test.go` | 新增 `listInterfaces` 处理器单测 |
| 前端 api | `web/src/api.js` | 新增 `getInterfaces()` |
| 前端视图 | `web/src/views/Settings.vue` | `rawIface` 文本输入→`<select>` + `loadInterfaces()` + 静默降级 |

约束（来自设计文档）：
- 后端端点受 `authRequired` 保护（与 `/api/config` 同组，无需额外处理）。
- 不排除任何接口（含 `lo`、容器 overlay 网卡），顺序保持 `net.Interfaces()` 返回顺序。
- 前端保留「自动（默认）」空选项；接口列表获取失败时静默降级（仅「自动」选项），不与配置加载错误混淆。
- 不修改配置存储结构 / scan 抓包实现 / `PUT /api/config` 持久化逻辑。

---

## Task 1 — 后端：`listInterfaces` 处理器与路由（TDD）

**1.1 新增处理器**（`internal/server/config.go`）：

```go
// ifaceInfo 描述一个网络接口及其地址（CIDR 形式）
type ifaceInfo struct {
	Name  string   `json:"name"`
	Addrs []string `json:"addrs"`
}

// listInterfaces 返回当前后端实例主机的所有网络接口，供前端下拉选择
func (s *Server) listInterfaces(c *gin.Context) {
	ifaces, err := net.Interfaces()
	if err != nil {
		c.JSON(500, gin.H{"error": "获取网络接口失败: " + err.Error()})
		return
	}
	out := make([]ifaceInfo, 0, len(ifaces))
	for _, itf := range ifaces {
		addrs := []string{}
		if as, err := itf.Addrs(); err == nil {
			for _, a := range as {
				addrs = append(addrs, a.String())
			}
		}
		out = append(out, ifaceInfo{Name: itf.Name, Addrs: addrs})
	}
	c.JSON(200, out)
}
```

在文件顶部 import 中加入 `"net"`（现有 import 块已有 `encoding/json` 与 `github.com/gin-gonic/gin`）。

**1.2 注册路由**（`registerConfig` 内）：

```go
func (s *Server) registerConfig(g *gin.RouterGroup) {
	g.GET("/config", s.getConfig)
	g.PUT("/config", s.updateConfig)
	g.GET("/interfaces", s.listInterfaces) // 新增
}
```

**1.3 测试**（`internal/server/config_test.go`，新增 `TestListInterfaces`）：

`listInterfaces` 不依赖 `s.deps`，用空白 `&Server{}` + `gin.CreateTestContext(httptest.NewRecorder())` 直接调用即可绕过鉴权：

```go
func TestListInterfaces(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	s := &Server{}
	s.listInterfaces(c)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	var got []ifaceInfo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("响应非 JSON 数组: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("期望至少返回一个接口")
	}
	for _, i := range got {
		if i.Name == "" {
			t.Fatal("接口 name 不得为空")
		}
	}
}
```

需在测试文件 import 加入 `"encoding/json"`、`"net/http/httptest"`、`"net/http"`（如尚未存在）。

**验收**：`go test ./internal/server/ -run TestListInterfaces -v` 通过；`go build ./...` 通过。

---

## Task 2 — 前端：api 层 `getInterfaces()`

在 `web/src/api.js` 的 `api` 对象中新增（紧跟 `getConfig`/`updateConfig`）：

```js
getInterfaces: () => request('GET', '/api/config/interfaces'),
```

**验收**：`web/src/api.js` 语法正确（`node -e "require('./web/src/api.js')"` 不适用 ESM，可跳过；以 Task 3 联调为准）。

---

## Task 3 — 前端：Settings.vue 文本框改下拉框

**3.1 状态**：在 `<script setup>` 中新增 `interfaces` ref 与 `loadInterfaces`：

```js
const interfaces = ref([])

async function loadInterfaces() {
  try {
    const list = await api.getInterfaces()
    if (Array.isArray(list)) interfaces.value = list
  } catch (e) {
    // 静默降级：interfaces 保持为空 → 下拉仅「自动（默认）」选项
    interfaces.value = []
  }
}
```

**3.2 加载时机**：`onMounted` 改为并行加载配置与接口列表：

```js
onMounted(() => {
  load()
  loadInterfaces()
})
```

（保留 `load()` 内的 `error` 处理，接口失败不污染 `error`。）

**3.3 模板替换**：将 `rawIface` 的 `<input type="text">`（第 48–52 行）替换为：

```html
<label class="field">
  <span class="field-label">抓包网卡 (RawIface)</span>
  <select class="field-input" v-model="form.rawIface">
    <option value="">自动（默认，留空）</option>
    <option v-for="i in interfaces" :key="i.name" :value="i.name">
      {{ i.name }}{{ i.addrs && i.addrs.length ? ' (' + i.addrs[0] + ')' : '' }}
    </option>
  </select>
  <span class="field-hint">raw 模式(SYN/ACK/FIN/Null/Xmas)抓包所用网卡；当前后端实例主机可用接口如下（通常选非 lo 的出口网卡）</span>
</label>
```

`form.rawIface` 仍由 `load()` 回填（第 119 行不变），`save()` 提交逻辑不变（第 137 行 `raw_iface: form.rawIface`）。

**验收**：
- `node --check web/src/api.js` 与 Vue 单文件构建无语法错误（由 Task 4 构建覆盖）。
- 手动：设置页下拉渲染接口列表（含 `lo` 与出口网卡 + IP 提示），选某网卡保存后重新进入正确回填；模拟 `/interfaces` 失败（如断网）时页面仍可用、仅「自动」选项。

---

## Task 4 — 构建与联调验证

- 后端：`go build ./...` 与 `go test ./internal/server/ -run TestListInterfaces` 通过。
- 前端：在 `web/` 执行构建（如 `npm run build` 或对应脚手架命令）无错误。
- 集成（可选，需运行环境）：`docker-compose up --build -d` 后访问设置页，确认下拉框列出后端实例网卡。

---

## 执行移交

计划已写入 `docs/superpowers/plans/2026-07-28-config-iface-select.md`。两种执行方式可选：

1. **子代理驱动（推荐）**：把上述 4 个 Task 拆成独立 issue，由子代理并行/串行执行后端与前端改动，最后统一构建验证。适合希望我后台批量完成。
2. **内联执行**：我在当前会话中按 Task 1→4 顺序直接改文件并验证。

请选择哪种方式继续？
