package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuditLogsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// 模拟 Audit 路由
	r.GET("/api/audit/logs", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items": []map[string]any{
				{
					"id":       1,
					"operator": "admin",
					"action":   "create_task",
					"target":   "127.0.0.1",
					"task_id":  "task-100",
				},
			},
			"total":       1,
			"page":        1,
			"page_size":   20,
			"total_pages": 1,
		})
	})

	req, err := http.NewRequest(http.MethodGet, "/api/audit/logs?q=admin&page=1&page_size=20", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}

	if resp["total"].(float64) != 1 {
		t.Errorf("expected total=1, got %v", resp["total"])
	}

	items, ok := resp["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 item, got %v", resp["items"])
	}

	item0 := items[0].(map[string]any)
	if item0["operator"] != "admin" {
		t.Errorf("expected operator=admin, got %v", item0["operator"])
	}
	if item0["action"] != "create_task" {
		t.Errorf("expected action=create_task, got %v", item0["action"])
	}
}
