package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
      "ip":       { "type": "ip" },
      "port":     { "type": "integer" },
      "proto":    { "type": "keyword" },
      "service":  { "type": "keyword" },
	  "state":    { "type": "text", "fields": { "keyword": { "type": "keyword", "ignore_above": 256 } } },
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
      "first_seen":{ "type": "date" },
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

// UpdateAsset 合并更新资产文档，使用脚本化更新确保 first_seen 仅创建时设置。
// doc 中应包含所有要写入的字段，first_seen 会在首次创建时自动设置，
// 后续更新不会覆盖已有 first_seen。
func (e *ESClient) UpdateAsset(ctx context.Context, id string, doc map[string]any) error {
	fs, hasFS := doc["first_seen"]

	// 浅拷贝 doc，避免修改调用方传入的 map（消除副作用与并发冲突）
	docParams := make(map[string]any, len(doc))
	upsertDoc := make(map[string]any, len(doc))
	for k, v := range doc {
		upsertDoc[k] = v
		if k != "first_seen" {
			docParams[k] = v
		}
	}

	scriptSource := "for (entry in params.doc.entrySet()) { ctx._source[entry.getKey()] = entry.getValue() }"
	paramsMap := map[string]any{
		"doc": docParams,
	}
	if hasFS {
		scriptSource = "if (ctx._source.first_seen == null && params.fs != null) { ctx._source.first_seen = params.fs } " + scriptSource
		paramsMap["fs"] = fs
	}

	bodyMap := map[string]any{
		"script": map[string]any{
			"source": scriptSource,
			"lang":   "painless",
			"params": paramsMap,
		},
		"upsert":          upsertDoc,
		"scripted_upsert": true,
	}

	body, err := json.Marshal(bodyMap)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s/_update/%s", e.addr, e.index, id), bytes.NewReader(body))
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
		// 409 版本冲突通常无害
		if resp.StatusCode == http.StatusConflict {
			return nil
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var errResp struct {
			Error struct {
				Reason string `json:"reason"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Reason != "" {
			return fmt.Errorf("es update status %d for id %s: %s", resp.StatusCode, id, errResp.Error.Reason)
		}
		return fmt.Errorf("es update status %d for id %s: %s", resp.StatusCode, id, string(respBody))
	}
	return nil
}

// UpsertAsset 写入/更新资产文档（兼容旧接口，使用 PUT 全量替换）。
// 注意：此方法会覆盖整个 _source，不保留未携带的字段。
// 推荐使用 UpdateAsset 替代。
func (e *ESClient) UpsertAsset(ctx context.Context, id string, doc map[string]any) error {
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
		Found  bool           `json:"found"`
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
		fmt.Sprintf("%s/%s/_doc/%s?refresh=wait_for", e.addr, e.index, id), nil)
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

// DeleteByQuery deletes all documents matching a structured Elasticsearch query.
func (e *ESClient) DeleteByQuery(ctx context.Context, query map[string]any) (int64, error) {
	body, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s/_delete_by_query?conflicts=proceed&refresh=true", e.addr, e.index), bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return 0, fmt.Errorf("es delete by query status %d: %s", resp.StatusCode, string(respBody))
	}
	var out struct {
		Deleted int64 `json:"deleted"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.Deleted, nil
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

// SearchAgg 发送带 aggs 聚合的 ES 查询，返回全量 ES HTTP 响应 Map（包含 aggregations 节点）
func (e *ESClient) SearchAgg(ctx context.Context, query map[string]any) (map[string]any, error) {
	body, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s/_search", e.addr, e.index), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("es search status %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
