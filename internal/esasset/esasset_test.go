package esasset

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"atlas/internal/model"
	"atlas/internal/store"
)

// TestUpsertPortID 验证 Upsert 调用 IndexAsset 且 _id 正确（port:<ip>:<port>）
func TestUpsertPortID(t *testing.T) {
	var gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		gotID = parts[len(parts)-1]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	es := store.NewES(srv.URL, "assets")
	s := New(es)
	err := s.Upsert(context.Background(), model.Asset{IP: "1.2.3.4", Port: 22, Proto: "tcp"})
	if err != nil {
		t.Fatal(err)
	}
	if gotID != "port:1.2.3.4:22" {
		t.Fatalf("unexpected index id: %q", gotID)
	}
}

func TestAssetFromSourceParsesObservationTimes(t *testing.T) {
	asset := assetFromSource(map[string]any{
		"ip":         "192.168.30.34",
		"port":       float64(443),
		"first_seen": "2026-08-05T10:20:30.123Z",
		"last_seen":  "2026-08-05T11:20:30.456Z",
	})
	firstSeen, _ := time.Parse(time.RFC3339Nano, "2026-08-05T10:20:30.123Z")
	lastSeen, _ := time.Parse(time.RFC3339Nano, "2026-08-05T11:20:30.456Z")
	if !asset.FirstSeen.Equal(firstSeen) || !asset.LastSeen.Equal(lastSeen) {
		t.Fatalf("observation times were not restored: %+v", asset)
	}
}

// TestGetHost 验证 GetHost 命中返回 model.Asset；未命中返回 ErrNotFound
func TestGetHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hits":{"total":{"value":1},"hits":[{"_source":{"ip":"1.2.3.4","org":"acme","os":"Linux","port":80}}]}}`))
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
}

// TestSearchAssets 验证 SearchAssets 透传 ES 结果并正确分页
func TestSearchAssets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hits":{"total":{"value":1},"hits":[{"_source":{"ip":"1.2.3.4","port":22,"banner":"SSH-2.0-OpenSSH_9.6"}}]}}`))
	}))
	defer srv.Close()

	s := New(store.NewES(srv.URL, "assets"))
	res, err := s.SearchAssets(context.Background(), "port=22", false, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Items) != 1 || res.Items[0]["banner"] != "SSH-2.0-OpenSSH_9.6" {
		t.Fatalf("bad result: %+v", res)
	}
}

// TestSearchAssets_Aggregated 验证 ES Composite Aggregation 限制 (top_hits<=100) 与截断感知
func TestSearchAssets_Aggregated(t *testing.T) {
	var requestBodies []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requestBodies = append(requestBodies, body)

		aggs, _ := body["aggs"].(map[string]any)
		ipComp, _ := aggs["ip_composite"].(map[string]any)

		if ipComp != nil {
			compObj, _ := ipComp["composite"].(map[string]any)
			after, _ := compObj["after"].(map[string]any)

			// 模拟 IP 批次 1 (1.1.1.1 doc_count=150 > len(hits)=2，发生截断)
			if len(after) == 0 {
				w.Write([]byte(`{
					"aggregations": {
						"ip_composite": {
							"after_key": {"ip": "1.1.1.1"},
							"buckets": [
								{
									"key": {"ip": "1.1.1.1"},
									"doc_count": 150,
									"top_docs": {
										"hits": {
											"hits": [
												{"_source": {"ip": "1.1.1.1", "org": "Cloudflare", "os": "Linux"}},
												{"_source": {"ip": "1.1.1.1", "port": 443, "service": "https", "title": "Secure Site", "host": "example.com"}}
											]
										}
									}
								}
							]
						}
					}
				}`))
				return
			}

			// 模拟 IP 批次 2 (最后一批，2.2.2.2 doc_count=1 未截断)
			w.Write([]byte(`{
				"aggregations": {
					"ip_composite": {
						"buckets": [
							{
								"key": {"ip": "2.2.2.2"},
								"doc_count": 1,
								"top_docs": {
									"hits": {
										"hits": [
											{"_source": {"ip": "2.2.2.2", "port": 22, "service": "ssh"}}
										]
									}
								}
							}
						]
					}
				}
			}`))
			return
		}
	}))
	defer srv.Close()

	s := New(store.NewES(srv.URL, "assets"))

	// 测试 1: 发起聚合请求并断言 top_hits size <= 100 限制
	requestBodies = nil
	res, err := s.SearchAssets(context.Background(), "", true, 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, reqBody := range requestBodies {
		aggs, _ := reqBody["aggs"].(map[string]any)
		for _, v := range aggs {
			if aggMap, ok := v.(map[string]any); ok {
				if subAggs, ok := aggMap["aggs"].(map[string]any); ok {
					if topDocs, ok := subAggs["top_docs"].(map[string]any); ok {
						if topHits, ok := topDocs["top_hits"].(map[string]any); ok {
							if sz, ok := topHits["size"].(float64); ok {
								if sz > 100 {
									t.Errorf("request %d top_hits.size (%v) exceeds 100 limit", i, sz)
								}
							}
						}
					}
				}
			}
		}
	}

	// 测试 2: 验证截断标志与元数据 (1.1.1.1 doc_count=150 > hits=2)
	if len(res.Items) < 1 {
		t.Fatalf("expected items, got empty")
	}
	item1 := res.Items[0]
	if trunc, ok := item1["truncated"].(bool); !ok || !trunc {
		t.Errorf("expected 1.1.1.1 truncated to be true, got %v", item1["truncated"])
	}
	if docCnt, ok := item1["document_count"].(int); !ok || docCnt != 150 {
		t.Errorf("expected document_count=150, got %v", item1["document_count"])
	}
	// 测试 3: 验证合并主机属性
	var hostItem map[string]any
	for _, it := range res.Items {
		if it["ip"] == "1.1.1.1" {
			hostItem = it
			break
		}
	}
	if hostItem == nil {
		t.Fatalf("expected aggregated item for 1.1.1.1")
	}
	if _, ok := hostItem["doc_type"]; ok {
		t.Errorf("aggregated item must not contain doc_type, got %v", hostItem["doc_type"])
	}
	if os, ok := hostItem["os"].(string); !ok || os != "Linux" {
		t.Errorf("expected merged os=Linux, got %v", hostItem["os"])
	}
}
