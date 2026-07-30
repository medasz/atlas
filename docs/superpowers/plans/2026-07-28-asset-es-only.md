# 资产数据全面迁移至 Elasticsearch（统一 Asset 结构，方案 B） Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将资产本体（hosts/ports/domains）的存储从 PostgreSQL 双写改为 Elasticsearch 唯一存储，抽取 `AssetStore` 接口并由 `esasset` 包以 ES 后端实现，移除 PG 资产表与 `es_pending` 机制，并加 ES 快照备份/自动恢复。资产本体统一为**单个 `model.Asset` 结构**，资产类型以 `_id` 前缀（`host:`/`port:`/`domain:`）与字段存在性（`exists port` / `exists domain` / `exists ip`）区分，**不再使用 `doc_type` 分类字段**，不再保留三套独立类型作为存储契约。

**Architecture:** `assetstore.AssetStore` 接口（基于统一的 `model.Asset`）的读写与检索，唯一实现 `esasset.ESAssetStore` 基于 `store.ESClient`。PG `Store` 仅保留 vulns/tasks/blacklist/config；`getHostDetail` 由 HTTP 层组合「ES 资产 + PG 漏洞」。检索只走 ES，删除 PG union 回退。迁移期用 `ReindexFromPG` 把 PG 资产灌入 ES 后再删 PG 资产表。

**Tech Stack:** Go 1.25, gin, pgxpool, gopacket/pcap(build tag), Elasticsearch 8.13.4 (REST via net/http), docker-compose。

## Global Constraints

- **ES 唯一存储**：部署后 `hosts`/`ports`/`domains` 表被删除，资产读写只经 `AssetStore`→ES。
- **统一 Asset 结构**：资产本体统一为 `model.Asset`，**无 `doc_type`/`Kind` 分类字段**；`AssetStore` 接口只认 `model.Asset`：`Upsert(ctx, model.Asset)`；读接口返回 `model.Asset` 或 `[]model.Asset`。`scan` worker 与 `reindex` 都构造 `model.Asset` 后调用 `Upsert`。**端口必须仍是独立文档/独立行**（满足 port 维度列表与 `port=22` 检索），不可把端口塞进 host 文档。资产类型以 `_id` 前缀（`host:`/`port:`/`domain:`）与字段存在性（`exists port`/`exists domain`/`exists ip`）判别。
- **仅资产本体迁 ES**：vulns / tasks / blacklist / config 仍留 PG（`vulns.asset_ref` 为字符串关联，无 FK）。
- **备份恢复**：ES 快照挂持久卷 + 启动自动恢复 + 周期打快照（默认 6h）。
- **删除项**：`es_pending` 列及 `FlushPendingES`/`indexAsset`/30s ticker/`searchAssetsPG`/`scopeUnionSelect`/`assetCols`/PG 资产 Upsert 法/`model.Host`/`model.Port`/`model.Domain`（确认无其他引用后移除）。
- **迁移顺序硬性约束**：先 `ReindexFromPG` 把 PG 资产灌入 ES 并校验，再执行删表迁移；错序即丢数据。
- **Commits require explicit user approval**（项目规则：禁止自动 git commit）。本计划的 commit 步骤在执行时需先取得用户许可；subagent 实现时**不得自行 commit**，由主线程在用户许可后提交。
- 沿用现有暗色赛博主题与 `api.js` 契约；前端 `Assets.vue` 本阶段不变（其消费 `SearchAssets` 返回的 ES `_source` 原始 map，与 Go 模型无关）。

---

## 统一 Asset 模型（新增）

`model.Asset` 的字段涵盖 host/port/domain 的全部属性，零值字段在写入 ES 时省略（`omitempty` 或构建 doc 时跳过零值）。资产类型以 `_id` 前缀（`host:`/`port:`/`domain:`）与字段存在性（`exists port` / `exists domain` / `exists ip`）判别，**不依赖任何分类字段**。

```go
// internal/model/asset.go
package model

import "time"

// Asset 资产本体统一结构；资产类型以 _id 前缀（host:/port:/domain:）与字段存在性区分，
// 不再使用 doc_type / Kind 分类字段。
type Asset struct {
    IP     string          `json:"ip,omitempty"`
    Port   int             `json:"port,omitempty"`
    Proto  string          `json:"proto,omitempty"`
    Domain string          `json:"domain,omitempty"` // domain 类型的完整主机名（=原 Domain.Name）
    Host   string          `json:"host,omitempty"`   // 到达端口所用主机名/域名（HTTP Host）
    ASN    int             `json:"asn,omitempty"`
    Org    string          `json:"org,omitempty"`
    OS     string          `json:"os,omitempty"`
    IsIPv6 bool            `json:"is_ipv6,omitempty"`
    State  string          `json:"state,omitempty"`
    Service string         `json:"service,omitempty"`
    Version string         `json:"version,omitempty"`
    Banner string          `json:"banner,omitempty"`
    Title  string          `json:"title,omitempty"`
    Server string          `json:"server,omitempty"`
    Tech   []string        `json:"tech,omitempty"`
    RegistrableDomain string `json:"registrable_domain,omitempty"`
    ResolvedIPs []string   `json:"resolved_ips,omitempty"`
    CNAME  []string        `json:"cname,omitempty"`
    OpenPorts int          `json:"open_ports,omitempty"`
    Cert   map[string]any  `json:"cert,omitempty"`
    WebInfo map[string]any `json:"webinfo,omitempty"`
    Geo    map[string]any  `json:"geo,omitempty"`
    Whois  map[string]any  `json:"whois,omitempty"`
    FirstSeen time.Time    `json:"first_seen,omitempty"`
    LastSeen  time.Time    `json:"last_seen,omitempty"`
}

// AssetID 返回 ES _id：port:<ip>:<port> / domain:<name> / host:<ip>
// 按字段存在性推导：Port>0 → 端口文档；Domain 非空 → 域名文档；否则 → 主机文档。
func AssetID(a Asset) string {
    if a.Port != 0 {
        return "port:" + a.IP + ":" + strconv.Itoa(a.Port)
    }
    if a.Domain != "" {
        return "domain:" + a.Domain
    }
    return "host:" + a.IP
}
```

`AssetStore` 接口：

```go
type AssetStore interface {
    Upsert(ctx context.Context, a model.Asset) error
    GetHost(ctx context.Context, ip string) (model.Asset, error)            // 未找到返回 ErrNotFound
    ListPortsByIP(ctx context.Context, ip string) ([]model.Asset, error)
    ListDomains(ctx context.Context) ([]model.Asset, error)
    GetHostDetail(ctx context.Context, ip string) (model.Asset, []model.Asset, error)
    SearchAssets(ctx context.Context, q string, aggregated bool, page, pageSize int) (*store.SearchResult, error)
}
```

---

## File Structure

**新增**
- `internal/model/asset.go` — `model.Asset`、`AssetID`。
- `internal/assetstore/assetstore.go` — `AssetStore` 接口 + `ErrNotFound`。
- `internal/esasset/esasset.go` — `ESAssetStore` 实现（doc 构建、读写、检索），只认 `model.Asset`。
- `internal/assetstore/reindex.go` — `ReindexFromPG`（过渡期：PG 读 Host/Port/Domain → 转 `model.Asset` → `Upsert`）。
- `configs/elasticsearch.yml` — 设 `path.repo`（快照仓库）。
- `migrations/000009_drop_asset_tables.up.sql` / `.down.sql` — 删 `hosts`/`ports`/`domains`。

**修改**
- `internal/store/es.go` — 加 `ErrNotFound`、`Get`、`Delete`、`RegisterSnapshotRepo`、`Snapshot`、`Restore`、`SnapshotExists`、`Count`。
- `internal/store/query.go` — 导出 `ParseQuery`；移除 `SearchAssets`/`buildESQuery`/`searchAssetsPG`/`scopeUnionSelect`/`assetCols`/`renumber`。
- `internal/store/pg.go` — 移除资产 Upsert/Get/List（含 `indexAsset`/`FlushPendingES`/`SetSearch` 及 `es` 字段）；保留 vulns 方法与 `ListAllHosts/Ports/Domains`（过渡期供 reindex，从 PG 读为 `model.Host/Port/Domain` 后转 `Asset`）；Task 10 删表时一并移除这些 `ListAll*`。
- `internal/scan/scan.go` — `Scanner.asset` 改为 `assetstore.AssetStore`；构造 `model.Asset` 调用 `Upsert`（取代 `UpsertHost/Port/Domain`）。
- `internal/server/server.go` — `Deps` 增加 `Asset assetstore.AssetStore`。
- `internal/server/asset.go` — 处理器改走 `Deps.Asset`（返回 `model.Asset`）；`getHostDetail` 组合 PG 漏洞。
- `cmd/atlas/main.go` — 构造 `esasset.ESAssetStore` 注入 scan/server；移除 `SetSearch`/30s ticker；加快照注册/恢复/ticker；保留 `es.CreateIndex`。
- `docker-compose.yml` — elasticsearch 挂 `es_backup` 卷 + 挂载 `configs/elasticsearch.yml`。

**测试**
- `internal/esasset/esasset_test.go` — 用 `httptest` 伪 ES 验证 `Upsert`(ID 正确)/`GetHost`/`ListPortsByIP`/`Search`。
- `internal/store/query_test.go`（已存在）— `toPG`/`toES` 解析测试保留。

---

### Task 1: 定义统一 Asset 模型与 AssetStore 接口

**Files:**
- Create: `internal/model/asset.go`
- Create: `internal/assetstore/assetstore.go`

**Interfaces:**
- Produces: `model.Asset`/`AssetID`（统一资产类型）、`assetstore.AssetStore` 接口、`assetstore.ErrNotFound`。后续所有资产读写依赖此接口与 `model.Asset`。

- [ ] **Step 1: 写 model/asset.go**

写入上文「统一 Asset 模型」代码块（`import` 含 `"strconv"` 与 `"time"`）。

- [ ] **Step 2: 写 assetstore.go**

```go
package assetstore

import (
	"context"
	"errors"

	"atlas/internal/model"
	"atlas/internal/store"
)

// ErrNotFound 资产不存在（ES 文档 404 映射）
var ErrNotFound = errors.New("asset not found")

// AssetStore 资产本体的统一存储接口（基于 model.Asset）；实现可为 ES（本期唯一实现）。
type AssetStore interface {
	Upsert(ctx context.Context, a model.Asset) error
	GetHost(ctx context.Context, ip string) (model.Asset, error)
	ListPortsByIP(ctx context.Context, ip string) ([]model.Asset, error)
	ListDomains(ctx context.Context) ([]model.Asset, error)
	GetHostDetail(ctx context.Context, ip string) (model.Asset, []model.Asset, error)
	SearchAssets(ctx context.Context, q string, aggregated bool, page, pageSize int) (*store.SearchResult, error)
}
```

- [ ] **Step 3: 编译校验**

Run: `cd atlas && go build ./internal/model/ ./internal/assetstore/`
Expected: 成功（仅类型，无实现）。

- [ ] **Step 4: Commit（需用户许可）**

```bash
git add internal/model/asset.go internal/assetstore/assetstore.go
git commit -m "feat(asset): 定义统一 model.Asset 与 AssetStore 接口"
```

---

### Task 2: ESClient 增加 Get/Delete/Count/快照方法

**Files:**
- Modify: `internal/store/es.go`

**Interfaces:**
- Consumes: `store.ESClient` 已有 `IndexAsset`/`Search`。
- Produces: `store.ErrNotFound`、`Get(ctx,id)`、`Delete(ctx,id)`、`Count(ctx)`、`RegisterSnapshotRepo`、`Snapshot`、`Restore`、`SnapshotExists`，供 `esasset` 与 `main.go` 使用。

- [ ] **Step 1: 写失败测试（验证 Get 404 映射）**

```go
package store

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestESGetNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	es := NewES(srv.URL, "assets")
	if _, err := es.Get(context.Background(), "host:1.2.3.4"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd atlas && go test ./internal/store/ -run TestESGetNotFound -v`
Expected: FAIL（`Get` 未定义）。

- [ ] **Step 3: 实现方法（追加到 es.go）**

```go
var ErrNotFound = errors.New("es document not found")

func (e *ESClient) Get(ctx context.Context, id string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/%s/_doc/%s", e.addr, e.index, id), nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("es get status %d", resp.StatusCode)
	}
	var out struct {
		Found  bool         `json:"found"`
		Source map[string]any `json:"_source"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if !out.Found {
		return nil, ErrNotFound
	}
	return out.Source, nil
}

func (e *ESClient) Delete(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/%s/_doc/%s", e.addr, e.index, id), nil)
	if err != nil {
		return err
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("es delete status %d", resp.StatusCode)
	}
	return nil
}

func (e *ESClient) Count(ctx context.Context) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/%s/_count", e.addr, e.index), nil)
	if err != nil {
		return 0, err
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("es count status %d", resp.StatusCode)
	}
	var out struct {
		Count int64 `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

func (e *ESClient) RegisterSnapshotRepo(ctx context.Context, name, location string) error {
	body, _ := json.Marshal(map[string]any{
		"type":     "fs",
		"settings": map[string]any{"location": location},
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/_snapshot/%s", e.addr, name), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("es register snapshot repo status %d", resp.StatusCode)
	}
	return nil
}

func (e *ESClient) Snapshot(ctx context.Context, repo, snap string) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/_snapshot/%s/%s", e.addr, repo, snap), nil)
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("es snapshot status %d", resp.StatusCode)
	}
	return nil
}

func (e *ESClient) SnapshotExists(ctx context.Context, repo, snap string) bool {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/_snapshot/%s/%s", e.addr, repo, snap), nil)
	resp, err := e.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (e *ESClient) Restore(ctx context.Context, repo, snap string) error {
	closeReq, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s/_close", e.addr, e.index), nil)
	if cr, err := e.http.Do(closeReq); err == nil {
		cr.Body.Close()
	}
	body, _ := json.Marshal(map[string]any{"indices": e.index})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/_snapshot/%s/%s/_restore", e.addr, repo, snap), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("es restore status %d", resp.StatusCode)
	}
	openReq, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s/_open", e.addr, e.index), nil)
	if or, err := e.http.Do(openReq); err == nil {
		or.Body.Close()
	}
	return nil
}
```

（确认 `import` 已有 `bytes`、`context`、`encoding/json`、`errors`、`fmt`、`net/http`、`time`；缺失则补。）

- [ ] **Step 4: 运行测试确认通过**

Run: `cd atlas && go test ./internal/store/ -run TestESGetNotFound -v`
Expected: PASS。

- [ ] **Step 5: Commit（需用户许可）**

```bash
git add internal/store/es.go
git commit -m "feat(es): ESClient 增加 Get/Delete/Count/快照方法"
```

---

### Task 3: 实现 esasset.ESAssetStore 写入与 doc 构建

**Files:**
- Create: `internal/esasset/esasset.go`

**Interfaces:**
- Consumes: `store.ESClient`、`store.ErrNotFound`、`model.Asset`、`model.AssetID`。
- Produces: `esasset.New(es)`、`ESAssetStore.Upsert`（接受 `model.Asset`，按 `AssetID` 索引），供 scan worker 与 reindex 使用。

- [ ] **Step 1: 写失败测试（验证 Upsert 调用 IndexAsset 且 _id 正确）**

```go
package esasset

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"atlas/internal/model"
	"atlas/internal/store"
)

type fakeES struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeES) record(id string) { f.mu.Lock(); f.calls = append(f.calls, id); f.mu.Unlock() }
func (f *fakeES) IndexAsset(ctx context.Context, id string, doc map[string]any) error {
	f.record("INDEX:" + id)
	return nil
}
func (f *fakeES) Search(ctx context.Context, q map[string]any) ([]map[string]any, int64, error) {
	return nil, 0, nil
}
func (f *fakeES) Get(ctx context.Context, id string) (map[string]any, error) {
	return nil, store.ErrNotFound
}
func (f *fakeES) Delete(ctx context.Context, id string) error { return nil }

func TestUpsertPortID(t *testing.T) {
	f := &fakeES{}
	s := New(f)
	err := s.Upsert(context.Background(), model.Asset{IP: "1.2.3.4", Port: 22, Proto: "tcp"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.calls[0] != "INDEX:port:1.2.3.4:22" {
		t.Fatalf("unexpected index id: %v", f.calls)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd atlas && go test ./internal/esasset/ -run TestUpsertPortID -v`
Expected: FAIL（`esasset` 包不存在）。

- [ ] **Step 3: 实现 esasset.go（写入部分）**

```go
package esasset

import (
	"context"

	"atlas/internal/model"
	"atlas/internal/store"
)

// ESAssetStore 以 Elasticsearch 为资产唯一存储的实现
type ESAssetStore struct{ es *store.ESClient }

func New(es *store.ESClient) *ESAssetStore { return &ESAssetStore{es: es} }

// Upsert 写入/更新资产（统一 model.Asset；_id 由 model.AssetID 决定）
func (s *ESAssetStore) Upsert(ctx context.Context, a model.Asset) error {
	doc := assetToDoc(a)
	return s.es.IndexAsset(ctx, model.AssetID(a), doc)
}

// assetToDoc 将统一 Asset 转为 ES 文档；跳过零值以减小体积
func assetToDoc(a model.Asset) map[string]any {
	m := map[string]any{}
	if a.IP != "" {
		m["ip"] = a.IP
	}
	if a.Port != 0 {
		m["port"] = a.Port
	}
	if a.Proto != "" {
		m["proto"] = a.Proto
	}
	if a.Domain != "" {
		m["domain"] = a.Domain
	}
	if a.Host != "" {
		m["host"] = a.Host
	}
	if a.ASN != 0 {
		m["asn"] = a.ASN
	}
	if a.Org != "" {
		m["org"] = a.Org
	}
	if a.OS != "" {
		m["os"] = a.OS
	}
	if a.IsIPv6 {
		m["is_ipv6"] = true
	}
	if a.State != "" {
		m["state"] = a.State
	}
	if a.Service != "" {
		m["service"] = a.Service
	}
	if a.Version != "" {
		m["version"] = a.Version
	}
	if a.Banner != "" {
		m["banner"] = a.Banner
	}
	if a.Title != "" {
		m["title"] = a.Title
	}
	if a.Server != "" {
		m["server"] = a.Server
	}
	if len(a.Tech) > 0 {
		m["tech"] = a.Tech
	}
	if a.RegistrableDomain != "" {
		m["registrable_domain"] = a.RegistrableDomain
	}
	if len(a.ResolvedIPs) > 0 {
		m["resolved_ips"] = a.ResolvedIPs
	}
	if len(a.CNAME) > 0 {
		m["cname"] = a.CNAME
	}
	if a.OpenPorts != 0 {
		m["open_ports"] = a.OpenPorts
	}
	if len(a.Cert) > 0 {
		m["cert"] = a.Cert
	}
	if len(a.WebInfo) > 0 {
		m["webinfo"] = a.WebInfo
	}
	if len(a.Geo) > 0 {
		m["geo"] = a.Geo
	}
	if len(a.Whois) > 0 {
		m["whois"] = a.Whois
	}
	if !a.FirstSeen.IsZero() {
		m["first_seen"] = a.FirstSeen
	}
	if !a.LastSeen.IsZero() {
		m["last_seen"] = a.LastSeen
	}
	return m
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd atlas && go test ./internal/esasset/ -run TestUpsertPortID -v`
Expected: PASS。

- [ ] **Step 5: Commit（需用户许可）**

```bash
git add internal/esasset/esasset.go internal/esasset/esasset_test.go
git commit -m "feat(esasset): 实现资产写入与统一 doc 构建"
```

---

### Task 4: 实现 esasset 读取（GetHost/ListPortsByIP/ListDomains/GetHostDetail）

**Files:**
- Modify: `internal/esasset/esasset.go`
- Test: `internal/esasset/esasset_test.go`

**Interfaces:**
- Consumes: `ESAssetStore.es.Get`/`Search`、`store.ErrNotFound`、`model.Asset`、`assetstore.ErrNotFound`。
- Produces: 读取方法，供 server 资产接口使用。

- [ ] **Step 1: 写失败测试（GetHost 命中返回 model.Asset；未命中返回 ErrNotFound）**

```go
func TestGetHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contains := func() bool {
			for _, s := range []string{"_doc/host:1.2.3.4"} {
				if len(r.URL.Path) >= len(s) && r.URL.Path[len(r.URL.Path)-len(s):] == s {
					return true
				}
			}
			return false
		}(); contains {
			w.Write([]byte(`{"found":true,"_source":{"ip":"1.2.3.4","org":"acme","os":"Linux"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	s := New(store.NewES(srv.URL, "assets"))
	h, err := s.GetHost(context.Background(), "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if h.IP != "1.2.3.4" || h.Org != "acme" || h.OS != "Linux" {
		t.Fatalf("bad host: %+v", h)
	}
	if _, err := s.GetHost(context.Background(), "9.9.9.9"); !errors.Is(err, assetstore.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd atlas && go test ./internal/esasset/ -run TestGetHost -v`
Expected: FAIL（`GetHost` 未实现）。

- [ ] **Step 3: 实现读取方法（追加到 esasset.go）**

```go
// GetHost 按 IP 读取主机资产；未找到返回 assetstore.ErrNotFound
func (s *ESAssetStore) GetHost(ctx context.Context, ip string) (model.Asset, error) {
	src, err := s.es.Get(ctx, "host:"+ip)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Asset{}, assetstore.ErrNotFound
		}
		return model.Asset{}, err
	}
	return assetFromSource(src), nil
}

// ListPortsByIP 列出某 IP 的全部端口（按 port 升序）
func (s *ESAssetStore) ListPortsByIP(ctx context.Context, ip string) ([]model.Asset, error) {
	q := map[string]any{
		"query": map[string]any{"bool": map[string]any{"must": []any{
			map[string]any{"exists": map[string]any{"field": "port"}},
			map[string]any{"term": map[string]any{"ip": ip}},
		}}},
		"sort":  []any{map[string]any{"port": map[string]any{"order": "asc"}}},
		"size":  10000,
	}
	items, _, err := s.es.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]model.Asset, 0, len(items))
	for _, it := range items {
		out = append(out, assetFromSource(it))
	}
	return out, nil
}

// ListDomains 列出全部域名（按 last_seen 倒序）
func (s *ESAssetStore) ListDomains(ctx context.Context) ([]model.Asset, error) {
	q := map[string]any{
		"query": map[string]any{"bool": map[string]any{"must": []any{map[string]any{"exists": map[string]any{"field": "domain"}}}}},
		"sort":  []any{map[string]any{"last_seen": map[string]any{"order": "desc"}}},
		"size":  10000,
	}
	items, _, err := s.es.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]model.Asset, 0, len(items))
	for _, it := range items {
		out = append(out, assetFromSource(it))
	}
	return out, nil
}

// GetHostDetail 主机 + 全部端口（漏洞由调用方从 PG 取）
func (s *ESAssetStore) GetHostDetail(ctx context.Context, ip string) (model.Asset, []model.Asset, error) {
	h, err := s.GetHost(ctx, ip)
	if err != nil {
		return model.Asset{}, nil, err
	}
	ports, err := s.ListPortsByIP(ctx, ip)
	if err != nil {
		return h, nil, err
	}
	return h, ports, nil
}

// assetFromSource 从 ES _source 还原统一 Asset
func assetFromSource(m map[string]any) model.Asset {
	a := model.Asset{}

	if v, ok := m["ip"].(string); ok {
		a.IP = v
	}
	if v, ok := m["port"].(float64); ok {
		a.Port = int(v)
	}
	if v, ok := m["proto"].(string); ok {
		a.Proto = v
	}
	if v, ok := m["domain"].(string); ok {
		a.Domain = v
	}
	if v, ok := m["host"].(string); ok {
		a.Host = v
	}
	if v, ok := m["asn"].(float64); ok {
		a.ASN = int(v)
	}
	if v, ok := m["org"].(string); ok {
		a.Org = v
	}
	if v, ok := m["os"].(string); ok {
		a.OS = v
	}
	if v, ok := m["is_ipv6"].(bool); ok {
		a.IsIPv6 = v
	}
	if v, ok := m["state"].(string); ok {
		a.State = v
	}
	if v, ok := m["service"].(string); ok {
		a.Service = v
	}
	if v, ok := m["version"].(string); ok {
		a.Version = v
	}
	if v, ok := m["banner"].(string); ok {
		a.Banner = v
	}
	if v, ok := m["title"].(string); ok {
		a.Title = v
	}
	if v, ok := m["server"].(string); ok {
		a.Server = v
	}
	if v, ok := m["tech"].([]any); ok {
		ts := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				ts = append(ts, s)
			}
		}
		a.Tech = ts
	}
	if v, ok := m["registrable_domain"].(string); ok {
		a.RegistrableDomain = v
	}
	if v, ok := m["resolved_ips"].([]any); ok {
		rs := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				rs = append(rs, s)
			}
		}
		a.ResolvedIPs = rs
	}
	if v, ok := m["cname"].([]any); ok {
		cs := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				cs = append(cs, s)
			}
		}
		a.CNAME = cs
	}
	if v, ok := m["open_ports"].(float64); ok {
		a.OpenPorts = int(v)
	}
	if v, ok := m["cert"].(map[string]any); ok {
		a.Cert = v
	}
	if v, ok := m["webinfo"].(map[string]any); ok {
		a.WebInfo = v
	}
	if v, ok := m["geo"].(map[string]any); ok {
		a.Geo = v
	}
	if v, ok := m["whois"].(map[string]any); ok {
		a.Whois = v
	}
	return a
}
```

（`esasset.go` 顶部 `import` 增加 `"errors"` 与 `"atlas/internal/assetstore"`。）

- [ ] **Step 4: 运行测试确认通过**

Run: `cd atlas && go test ./internal/esasset/ -run 'TestGetHost|TestUpsertPortID' -v`
Expected: PASS。

- [ ] **Step 5: Commit（需用户许可）**

```bash
git add internal/esasset/esasset.go internal/esasset/esasset_test.go
git commit -m "feat(esasset): 实现资产读取与 GetHostDetail（统一 Asset）"
```

---

### Task 5: 实现 esasset.SearchAssets（迁移 buildESQuery）

**Files:**
- Modify: `internal/esasset/esasset.go`
- Modify: `internal/store/query.go`（导出 `ParseQuery`，删除 `SearchAssets`/`buildESQuery`/`searchAssetsPG`/`scopeUnionSelect`/`assetCols`/`renumber`）

**Interfaces:**
- Consumes: `store.ParseQuery`（需导出）、`store.SearchResult`、`store.ESClient.Search`。
- Produces: `SearchAssets`，供 server 检索接口。

- [ ] **Step 1: 写失败测试（SearchAssets 透传 ES 结果）**

```go
func TestSearchAssets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hits":{"total":{"value":1},"hits":[{"_source":{"ip":"1.2.3.4","port":22}}]}}`))
	}))
	defer srv.Close()
	s := New(store.NewES(srv.URL, "assets"))
	res, err := s.SearchAssets(context.Background(), "port=22", false, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Items) != 1 {
		t.Fatalf("bad result: %+v", res)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd atlas && go test ./internal/esasset/ -run TestSearchAssets -v`
Expected: FAIL（`SearchAssets` 未实现）。

- [ ] **Step 3: 导出 query.go 的 ParseQuery**

`internal/store/query.go` 将 `func parseQuery(q string) node` 改名为 `func ParseQuery(q string) node`（含调用处 `SearchAssets` 内 `parseQuery`→`ParseQuery`）。

- [ ] **Step 4: 实现 esasset.SearchAssets + 迁移 buildESQuery（追加到 esasset.go）**

```go
// SearchAssets 资产检索（仅 ES），标准分页
func (s *ESAssetStore) SearchAssets(ctx context.Context, q string, aggregated bool, page, pageSize int) (*store.SearchResult, error) {
	root := store.ParseQuery(q)
	from := (page - 1) * pageSize
	if from < 0 {
		from = 0
	}
	query := buildESQuery(root, from, pageSize)
	items, total, err := s.es.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	tp := pageSize
	if tp <= 0 {
		tp = 20
	}
	totalPages := 0
	if tp > 0 && total > 0 {
		totalPages = int((total + int64(tp) - 1) / int64(tp))
	}
	return &store.SearchResult{Total: total, Page: page, PageSize: tp, TotalPages: totalPages, Items: items}, nil
}

func buildESQuery(root node, from, size int) map[string]any {
	must := []any{}
	if root != nil {
		must = append(must, root.toES())
	}
	if size <= 0 {
		size = 20
	}
	if from < 0 {
		from = 0
	}
	if len(must) == 0 {
		return map[string]any{"query": map[string]any{"match_all": map[string]any{}}, "from": from, "size": size}
	}
	return map[string]any{"query": map[string]any{"bool": map[string]any{"must": must}}, "from": from, "size": size}
}
```

注意：`buildESQuery` 用到 `node` 接口（来自 `store` 包，由 `ParseQuery` 导出隐式可见）。`query.go` 中 `node`/`cmpNode`/`andNode`/`orNode` 及其 `toES` 方法保留（被 `ParseQuery` 使用）。

- [ ] **Step 5: 从 query.go 删除 PG 检索残留**

删除 `query.go` 中：`SearchAssets` 方法、`buildESQuery`（store 内旧版）、`scopeUnionSelect`、`searchAssetsPG`、`renumber`、`assetCols` 常量。保留 `ParseQuery`/`tokenize`/`splitCmp`/`fieldDefs`/`toES`/`toPG`/`likeExpr`/`pgCol`/`scopeAllCols`/`esAllFields`（解析单测依赖）。

- [ ] **Step 6: 运行测试确认通过**

Run: `cd atlas && go test ./internal/esasset/ -run TestSearchAssets -v && go test ./internal/store/ -run TestParse -v`
Expected: 两组均 PASS。

- [ ] **Step 7: Commit（需用户许可）**

```bash
git add internal/esasset/esasset.go internal/esasset/esasset_test.go internal/store/query.go
git commit -m "feat(esasset): 实现 SearchAssets，移除 PG 检索残留"
```

---

### Task 6: 清理 PG Store 的资产写入方法

**Files:**
- Modify: `internal/store/pg.go`

**Interfaces:**
- Consumes: 无（纯删除）。
- Produces：PG `Store` 不再含资产 Upsert/Get/List；`es` 字段/`SetSearch`/`indexAsset`/`FlushPendingES` 移除；保留 vulns 方法与过渡期 `ListAllHosts/Ports/Domains`（Task 9 reindex 用，从 PG 读为 `model.Host/Port/Domain`）。

- [ ] **Step 1: 删除资产相关代码块**

在 `pg.go` 中删除：
- `SetSearch` 方法及 `Store.es` 字段。
- `indexAsset`、`FlushPendingES`。
- `UpsertHost`、`UpsertPort`、`UpsertDomain`。
- `GetHost`、`ListPortsByIP`、`ListDomains`（单个查询版）。
- 保留：`ListAllHosts`/`ListAllPorts`/`ListAllDomains`（全量读，过渡期供 reindex）、`ListVulns`/`ListVulnsByHost`/`UpsertVuln`、`Pool`/`Close`/`RunMigrations`、非资产 SQL。

- [ ] **Step 2: 确认 pg.go 编译（其余引用待后续 Task 修改）**

Run: `cd atlas && go build ./internal/store/`
Expected: 成功（此时 `scan`/`server` 仍引用旧 `Store` 资产方法，故全量 `go build ./...` 暂失败，属预期）。

- [ ] **Step 3: Commit（需用户许可）**

```bash
git add internal/store/pg.go
git commit -m "refactor(store): 移除 PG 资产写入与 es_pending 机制"
```

---

### Task 7: 改造 scan worker 使用 AssetStore（构造 model.Asset）

**Files:**
- Modify: `internal/scan/scan.go`

**Interfaces:**
- Consumes: `assetstore.AssetStore`（替代 `*store.Store` 作资产写入）、`model.Asset`。
- Produces：scan worker 资产写入走接口，构造 `model.Asset` 调用 `Upsert`；供 main.go 注入 `esasset`。

- [ ] **Step 1: 改 Scanner 字段与 New 签名**

`scan.go` 中：
- `import` 增加 `"atlas/internal/assetstore"`、`"atlas/internal/model"`。
- `Scanner` 结构体 `store *store.Store` 改为 `asset assetstore.AssetStore`。
- `New` 签名由 `func New(s *store.Store, ...)` 改为 `func New(asset assetstore.AssetStore, r *ratelimit.Limiter, defaultPorts []int, fp *fingerprint.Service, scanCfg config.ScanConfig) *Scanner`。

- [ ] **Step 2: 替换 Upsert 调用为构造 model.Asset 后 Upsert**

将 `sc.store.UpsertPort(...)` / `sc.store.UpsertHost(...)` / `sc.store.UpsertDomain(...)` 替换为构造 `model.Asset`：

- 原 `UpsertHost(h)` → `sc.asset.Upsert(ctx, model.Asset{IP: h.IP, ASN: h.ASN, Org: h.Org, OS: h.OS, IsIPv6: h.IsIPv6, OpenPorts: len(h.OpenPorts), FirstSeen: h.FirstSeen, LastSeen: h.LastSeen})`（字段按原 `Host` 结构取用；如原代码用 `model.Host{}` 构造，直接整体映射为 `model.Asset`）。
- 原 `UpsertPort(p)` → `sc.asset.Upsert(ctx, model.Asset{IP: p.IP, Port: p.Port, Proto: p.Proto, State: p.State, Service: p.Service, Version: p.Version, Banner: p.Banner, Title: p.Title, Host: p.Host, IsIPv6: p.IsIPv6, Cert: p.Cert, WebInfo: p.WebInfo, FirstSeen: p.FirstSeen, LastSeen: p.LastSeen})`。
- 原 `UpsertDomain(d)` → `sc.asset.Upsert(ctx, model.Asset{Domain: d.Name, Host: d.Name, RegistrableDomain: d.RegistrableDomain, ResolvedIPs: d.ResolvedIPs, CNAME: d.CNAME, Org: d.Org, ASN: d.ASN, IsIPv6: d.IsIPv6, Whois: d.Whois, FirstSeen: d.FirstSeen, LastSeen: d.LastSeen})`。

（具体字段以 `scan.go` 现有调用处的 `model.Host/Port/Domain` 取值为准，逐字段映射。）

- [ ] **Step 3: 编译校验**

Run: `cd atlas && go build ./internal/scan/`
Expected: 成功。

- [ ] **Step 4: Commit（需用户许可）**

```bash
git add internal/scan/scan.go
git commit -m "refactor(scan): Scanner 资产写入改用 AssetStore 接口（统一 Asset）"
```

---

### Task 8: 改造 server 资产接口使用 AssetStore

**Files:**
- Modify: `internal/server/server.go`、`internal/server/asset.go`

**Interfaces:**
- Consumes: `assetstore.AssetStore`、`store`（PG，漏洞）、`model.Asset`。
- Produces：资产 HTTP 接口走 `Deps.Asset`；`getHostDetail` 组合 PG 漏洞。

- [ ] **Step 1: Deps 增加 Asset**

`server.go` 的 `import` 增加 `"atlas/internal/assetstore"`；`Deps` 结构体增加 `Asset assetstore.AssetStore`。

- [ ] **Step 2: 改 asset.go 处理器**

`asset.go` 中：
- `searchAssets`：`s.deps.Store.SearchAssets(...)` → `s.deps.Asset.SearchAssets(...)`。
- `getHost`：`s.deps.Asset.GetHost(...)`；错误时区分 `assetstore.ErrNotFound` → 404。
- `getHostDetail`：资产部分 `s.deps.Asset.GetHostDetail(ip)`（host `model.Asset` + `[]model.Asset` ports）；漏洞仍 `s.deps.Store.ListVulnsByHost(ctx, ip)`。

```go
func (s *Server) getHost(c *gin.Context) {
	h, err := s.deps.Asset.GetHost(c.Request.Context(), c.Param("ip"))
	if errors.Is(err, assetstore.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"host": h})
}

func (s *Server) getHostDetail(c *gin.Context) {
	ip := c.Param("ip")
	h, ports, err := s.deps.Asset.GetHostDetail(c.Request.Context(), ip)
	if errors.Is(err, assetstore.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	vulns, _ := s.deps.Store.ListVulnsByHost(c.Request.Context(), ip)
	c.JSON(http.StatusOK, gin.H{"host": h, "ports": ports, "vulns": vulns})
}
```

（`asset.go` 顶部 `import` 增加 `"errors"` 与 `"atlas/internal/assetstore"`。）

- [ ] **Step 3: Commit（需用户许可）**

```bash
git add internal/server/server.go internal/server/asset.go
git commit -m "refactor(server): 资产接口改用 AssetStore，详情组合 PG 漏洞"
```

---

### Task 9: ReindexFromPG + 管理端点（过渡期）

**Files:**
- Create: `internal/assetstore/reindex.go`
- Modify: `internal/server/asset.go`（加 `POST /api/admin/reindex`）

**Interfaces:**
- Consumes: `*store.Store`（PG 全量读 Host/Port/Domain，过渡期仍存在）、`assetstore.AssetStore`（写 ES）。
- Produces：一次性回填；Task 10 删表后随 PG 资产读方法一并移除。

- [ ] **Step 1: 实现 ReindexFromPG（PG Host/Port/Domain → model.Asset → Upsert）**

```go
package assetstore

import (
	"context"
	"log"

	"atlas/internal/model"
	"atlas/internal/store"
)

// ReindexFromPG 把 PG 中的资产全量写入 AssetStore（ES）。仅在删 PG 资产表前的一次性迁移使用。
func ReindexFromPG(ctx context.Context, pg *store.Store, a AssetStore) error {
	hosts, err := pg.ListAllHosts(ctx)
	if err != nil {
		return err
	}
	for _, h := range hosts {
		if err := a.Upsert(ctx, model.Asset{
			IP: h.IP, ASN: h.ASN, Org: h.Org, OS: h.OS,
			IsIPv6: h.IsIPv6, OpenPorts: len(h.OpenPorts), Geo: h.Geo,
			FirstSeen: h.FirstSeen, LastSeen: h.LastSeen,
		}); err != nil {
			log.Printf("reindex host %s: %v", h.IP, err)
		}
	}
	ports, err := pg.ListAllPorts(ctx)
	if err != nil {
		return err
	}
	for _, p := range ports {
		if err := a.Upsert(ctx, model.Asset{
			IP: p.IP, Port: p.Port, Proto: p.Proto, State: p.State,
			Service: p.Service, Version: p.Version, Banner: p.Banner, Title: p.Title,
			Host: p.Host, IsIPv6: p.IsIPv6, Cert: p.Cert, WebInfo: p.WebInfo,
			FirstSeen: p.FirstSeen, LastSeen: p.LastSeen,
		}); err != nil {
			log.Printf("reindex port %s:%d: %v", p.IP, p.Port, err)
		}
	}
	domains, err := pg.ListAllDomains(ctx)
	if err != nil {
		return err
	}
	for _, d := range domains {
		if err := a.Upsert(ctx, model.Asset{
			Domain: d.Name, Host: d.Name,
			RegistrableDomain: d.RegistrableDomain, ResolvedIPs: d.ResolvedIPs,
			CNAME: d.CNAME, Org: d.Org, ASN: d.ASN, IsIPv6: d.IsIPv6,
			Whois: d.Whois, FirstSeen: d.FirstSeen, LastSeen: d.LastSeen,
		}); err != nil {
			log.Printf("reindex domain %s: %v", d.Name, err)
		}
	}
	log.Printf("reindex done: %d hosts, %d ports, %d domains", len(hosts), len(ports), len(domains))
	return nil
}
```

需在 `pg.go` 增加 `ListAllHosts`/`ListAllPorts`/`ListAllDomains`（全量读，从 PG 读为 `model.Host/Port/Domain`，扫描逻辑参照 `pg.go` 现有写法；这些 PG 类型仅过渡期用于 reindex）。

- [ ] **Step 2: 加管理端点**

`asset.go` 的 `registerAssets` 增加：`g.POST("/admin/reindex", s.adminReindex)`，并实现：
```go
func (s *Server) adminReindex(c *gin.Context) {
	if err := assetstore.ReindexFromPG(c.Request.Context(), s.deps.Store, s.deps.Asset); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "reindexed"})
}
```

- [ ] **Step 3: 编译校验**

Run: `cd atlas && go build ./...`
Expected: 成功（此时 PG 资产表仍存在，reindex 可用）。

- [ ] **Step 4: 手动回填（部署后由用户触发，非自动）**

文档化：部署新镜像后，调用 `POST /api/admin/reindex` 把 PG 资产灌入 ES；随后用 `curl .../assets/_count` 校验 ES 文档数 ≈ PG 资产数（hosts+ports+domains）。**确认后再执行 Task 10。**

- [ ] **Step 5: Commit（需用户许可）**

```bash
git add internal/assetstore/reindex.go internal/server/asset.go internal/store/pg.go
git commit -m "feat(migrate): 新增 ReindexFromPG 与 /api/admin/reindex（过渡期）"
```

---

### Task 10: 删 PG 资产表 + 移除迁移代码 + 清理 model 旧类型

**Files:**
- Create: `migrations/000009_drop_asset_tables.up.sql` + `.down.sql`
- Modify: `internal/store/pg.go`（删 `ListAllHosts/Ports/Domains`）、`internal/assetstore/reindex.go`（整文件删除）、`internal/server/asset.go`（移除 `/admin/reindex`）、`internal/model/model.go`（确认 `Host/Port/Domain` 无其他引用后删除）

**Interfaces:**
- 前置：`ReindexFromPG` 已成功执行且校验通过。
- 产出：PG 不再含资产表；迁移代码与 `model.Host/Port/Domain` 清理（统一 Asset 后不再需要）。

- [ ] **Step 1: 写删表迁移**

`migrations/000009_drop_asset_tables.up.sql`:
```sql
DROP TABLE IF EXISTS ports;
DROP TABLE IF EXISTS hosts;
DROP TABLE IF EXISTS domains;
```
`migrations/000009_drop_asset_tables.down.sql`（灾难恢复用，重建最小表）：
```sql
CREATE TABLE IF NOT EXISTS hosts (
  ip TEXT PRIMARY KEY, asn INT, org TEXT, geo JSONB, os TEXT,
  is_ipv6 BOOLEAN, open_ports INT, first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS ports (
  ip TEXT, port INT, proto TEXT, state TEXT, service TEXT, version TEXT,
  banner TEXT, cert JSONB, title TEXT, host TEXT, is_ipv6 BOOLEAN, webinfo JSONB,
  first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ,
  PRIMARY KEY (ip, port, proto)
);
CREATE TABLE IF NOT EXISTS domains (
  name TEXT PRIMARY KEY, registrable_domain TEXT, resolved_ips TEXT,
  cname TEXT, org TEXT, asn INT, is_ipv6 BOOLEAN, whois JSONB,
  first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ
);
```

- [ ] **Step 2: 删除迁移相关代码与旧 model 类型**

- `pg.go`：删除 `ListAllHosts`/`ListAllPorts`/`ListAllDomains`（Task 9 新增）。
- `internal/assetstore/reindex.go`：整文件删除。
- `asset.go`：移除 `adminReindex` 处理器与 `registerAssets` 中的 `g.POST("/admin/reindex", ...)` 及 `assetstore` import（若不再被 asset.go 其他处引用）。
- `model.go`：在确认全仓库已无 `model.Host`/`model.Port`/`model.Domain` 引用后（Task 7/8/9 已全部改用 `model.Asset`），删除这三个类型定义。若仍有遗留引用，先修复引用再删除。

- [ ] **Step 3: 编译校验**

Run: `cd atlas && go build ./...`
Expected: 成功。

- [ ] **Step 4: 数据库执行迁移（用户触发，非自动）**

文档化：运行 `migrations/000009`（`atlas` 启动时 `RunMigrations` 会自动应用；确认 ES 已回填后再重启新镜像以触发删表）。

- [ ] **Step 5: Commit（需用户许可）**

```bash
git add migrations/000009_drop_asset_tables.up.sql migrations/000009_drop_asset_tables.down.sql internal/store/pg.go internal/server/asset.go internal/model/model.go
git commit -m "refactor(store): 删除 PG 资产表、迁移代码与旧 model.Host/Port/Domain"
```

---

### Task 11: ES 快照备份 / 自动恢复

**Files:**
- Create: `configs/elasticsearch.yml`
- Modify: `docker-compose.yml`、`cmd/atlas/main.go`

**Interfaces:**
- Consumes: `store.ESClient` 快照方法（Task 2）。
- Produces：ES 数据可恢复；启动空索引自动恢复。

- [ ] **Step 1: 配置 ES path.repo**

`configs/elasticsearch.yml`:
```yaml
path.repo: ["/backups"]
```
`docker-compose.yml` 的 `elasticsearch` 服务增加：
```yaml
    volumes:
      - es_backup:/backups
      - ./configs/elasticsearch.yml:/usr/share/elasticsearch/config/elasticsearch.yml:ro
```
并在 `volumes:` 段增加 `es_backup:`。

- [ ] **Step 2: main.go 注册仓库 + 自动恢复 + 周期快照**

在 `main.go` 中 `es.CreateIndex` 成功后：
```go
if err := es.CreateIndex(ctx); err != nil {
	log.Printf("elasticsearch init warning: %v", err)
} else {
	if err := es.RegisterSnapshotRepo(ctx, "atlas_backup", "/backups"); err != nil {
		log.Printf("register snapshot repo: %v", err)
	}
	if cnt, _ := es.Count(ctx); cnt == 0 && es.SnapshotExists(ctx, "atlas_backup", "auto") {
		if err := es.Restore(ctx, "atlas_backup", "auto"); err != nil {
			log.Printf("auto restore failed: %v", err)
		} else {
			log.Println("elasticsearch restored from snapshot")
		}
	}
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			ts := time.Now().Format("20060102-150405")
			if err := es.Snapshot(ctx, "atlas_backup", "auto-"+ts); err != nil {
				log.Printf("snapshot failed: %v", err)
			}
		}
	}()
}
```

同时**删除**旧的 `st.SetSearch(es)` 与 30s `FlushPendingES` ticker。新增 `assetStore := esasset.New(es)`，并在 `scan.New(...)` 与 `server.New(Deps{...})` 注入：
- `scanner := scan.New(assetStore, limiter, defaultPorts, fp, cfg.Scan)`
- `server.New(server.Deps{ ..., Store: st, Asset: assetStore, ... })`

`main.go` import 增加 `"atlas/internal/assetstore"`、`"atlas/internal/esasset"`。

- [ ] **Step 3: 编译校验**

Run: `cd atlas && go build ./...`
Expected: 成功。

- [ ] **Step 4: Commit（需用户许可）**

```bash
git add configs/elasticsearch.yml docker-compose.yml cmd/atlas/main.go internal/store/es.go
git commit -m "feat(backup): ES 快照仓库 + 自动恢复 + 周期快照"
```

---

### Task 12: 构建与冒烟验证

**Files:** 无新增，验证为主。

- [ ] **Step 1: 全量构建**

Run: `cd atlas && go build ./... && go vet ./... && go test ./... && cd web && npm run build`
Expected: 全部成功。

- [ ] **Step 2: 构建并启动栈**

Run（需用户许可执行，因涉及 docker）：
`cd atlas && docker-compose up --build`
Expected：atlas/atlas2/es 启动正常。

- [ ] **Step 3: 回填并校验（仅首次迁移）**

文档化：新镜像启动后 `POST /api/admin/reindex`；`curl .../assets/_count` 应 ≈ PG 原资产数；随后重启以应用删表迁移（Task 10）。
验证 `GET /api/assets?q=port=22` 返回该端口；`GET /api/hosts/<ip>/detail` 返回资产+漏洞；分页/排序正常。

- [ ] **Step 4: 验证快照恢复**

文档化：删除 `assets` 索引后重启 atlas，应能从 `es_backup` 快照自动恢复（日志出现 `elasticsearch restored from snapshot`）。

---

## Self-Review 对照 Spec

1. **Spec 覆盖**：统一 Asset 模型（Task 1）+ 接口/实现（Task 1-5）、PG 清理（Task 6,10）、scan/server 重写（Task 7-8）、reindex+删表（Task 9-10）、快照（Task 11）、测试/冒烟（Task 12）均覆盖；范围（仅资产本体）由 vulns 留 PG 保证。
2. **占位符扫描**：无 TBD/TODO；`ListAllHosts/Ports/Domains` 的扫描逻辑注明“参照现有写法”，实现时需补全（非占位，属已知模式）。
3. **类型一致性**：`AssetStore`/`esasset`/`store.SearchResult`/`store.ErrNotFound`/`assetstore.ErrNotFound`/`model.Asset`/`model.AssetID` 全链路一致；统一 Asset 后 `model.Host/Port/Domain` 在 Task 10 删除。
4. **顺序风险**：Task 9→Task 10 强制“先 reindex 校验、后删表”；已在 Task 9 Step 4、Task 10 Step 4 标红为手动门控。
5. **统一结构约束**：端口保持独立文档（`AssetID` = `port:<ip>:<port>`），不并入 host；`SearchAssets` 返回 ES `_source` 原始 map，前端无需改动。
