package store

import (
	"bytes"
	"context"
	"encoding/json"
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
