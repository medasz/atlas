package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestListInterfaces 直接调用 listInterfaces 处理器验证返回结构。
// 注意：该 handler 仅依赖标准库 net，不读取 s.deps，故可用空白 &Server{} 绕过鉴权。
// 鉴权由 server.go 的 /api 组 authRequired 中间件静态保证（见 code review）。
func TestListInterfaces(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	s := &Server{}
	s.listInterfaces(c)
	if w.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("期望 Content-Type 含 application/json，实际 %q", ct)
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
