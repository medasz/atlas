package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ESClient Elasticsearch 客户端（基于 net/http，无额外依赖）
type ESClient struct {
	addr  string
	index string
	http  *http.Client
}

// NewES 创建 ES 客户端
func NewES(addr, index string) *ESClient {
	return &ESClient{addr: addr, index: index, http: &http.Client{Timeout: 10 * time.Second}}
}

// assetMapping assets 索引映射（对齐 SPEC §3.2）
const assetMapping = `{
  "mappings": {
    "properties": {
      "doc_type": { "type": "keyword" },
      "ip":       { "type": "ip" },
      "port":     { "type": "integer" },
      "proto":    { "type": "keyword" },
      "service":  { "type": "keyword" },
      "version":  { "type": "keyword" },
      "banner":   { "type": "text" },
      "title":    { "type": "text" },
      "tag":      { "type": "keyword" },
      "hostname": { "type": "keyword" },
      "host":     { "type": "keyword" },
      "name":     { "type": "keyword" },
      "registrable_domain": { "type": "keyword" },
      "server":   { "type": "keyword" },
      "tech":     { "type": "keyword" },
      "os":       { "type": "keyword" },
      "asn":      { "type": "integer" },
      "org":      { "type": "keyword" },
      "geo":      { "type": "object" },
      "is_ipv6":  { "type": "boolean" },
      "last_seen":{ "type": "date" }
    }
  }
}`

// CreateIndex 若索引不存在则按映射创建
func (e *ESClient) CreateIndex(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/%s", e.addr, e.index), bytes.NewReader([]byte(assetMapping)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 200 已创建；400 表示已存在（resource_already_exists），均视为成功
	if resp.StatusCode >= 300 && resp.StatusCode != 400 {
		return fmt.Errorf("es create index status %d", resp.StatusCode)
	}
	return nil
}

// IndexAsset 写入/更新资产文档（_id 由调用方保证唯一，如 ip:port）
func (e *ESClient) IndexAsset(ctx context.Context, id string, doc map[string]any) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/%s/_doc/%s", e.addr, e.index, id), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("es index status %d", resp.StatusCode)
	}
	return nil
}

// Search 执行 ES 查询，返回命中文档列表与总命中数（用于分页 total）
func (e *ESClient) Search(ctx context.Context, query map[string]any) ([]map[string]any, int64, error) {
	body, err := json.Marshal(query)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s/_search", e.addr, e.index), bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var out struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source map[string]any `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, err
	}
	items := make([]map[string]any, 0, len(out.Hits.Hits))
	for _, h := range out.Hits.Hits {
		items = append(items, h.Source)
	}
	return items, out.Hits.Total.Value, nil
}

// ErrNotFound 文档不存在（Get 404 映射）
var ErrNotFound = errors.New("es document not found")

// Get 按 _id 读取文档；404 或 found=false 返回 ErrNotFound
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

// Delete 按 _id 删除文档（忽略 404）
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

// Count 返回索引文档总数
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

// RegisterSnapshotRepo 注册 fs 类型快照仓库（已存在则忽略 400）
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

// Snapshot 对 assets 索引打快照
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

// SnapshotExists 判断指定快照是否存在
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

// Restore 从快照恢复 assets 索引。
// 流程：先删除同名索引（避免已存在索引导致恢复 400 冲突）→ 发起恢复（含 partial/
// include_global_state:false，避免全局状态冲突）→ 轮询至索引可读。恢复为异步操作，
// 必须等待其完成，不能在返回后立刻查询。
func (e *ESClient) Restore(ctx context.Context, repo, snap string) error {
	// 1) 删除可能已存在的同名索引（空索引场景也无妨）
	delReq, _ := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/%s", e.addr, e.index), nil)
	if dr, err := e.http.Do(delReq); err == nil {
		dr.Body.Close()
	}
	// 2) 发起恢复请求；partial+include_global_state:false 防止全局状态/分片冲突
	body, _ := json.Marshal(map[string]any{
		"indices":             e.index,
		"include_global_state": false,
		"partial":             true,
	})
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
	// 3) 恢复为异步：轮询直至索引可读（Count 不再返回错误即视为恢复完成）
	return e.waitForRestore(ctx)
}

// waitForRestore 轮询等待恢复后的索引变为可读（最多约 10s）。
func (e *ESClient) waitForRestore(ctx context.Context) error {
	for i := 0; i < 20; i++ {
		if _, err := e.Count(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("es restore wait timeout")
}

// ListSnapshots 列出仓库内的全部快照名称；调用方自行挑选（如最新的 auto-<时间戳>）
func (e *ESClient) ListSnapshots(ctx context.Context, repo string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/_snapshot/%s/_all", e.addr, repo), nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("es list snapshots status %d", resp.StatusCode)
	}
	var out struct {
		Snapshots []struct {
			Snapshot string `json:"snapshot"`
		} `json:"snapshots"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(out.Snapshots))
	for _, s := range out.Snapshots {
		names = append(names, s.Snapshot)
	}
	return names, nil
}
