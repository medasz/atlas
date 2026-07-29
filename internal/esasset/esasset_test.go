package esasset

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	err := s.Upsert(context.Background(), model.Asset{Kind: model.KindPort, IP: "1.2.3.4", Port: 22, Proto: "tcp"})
	if err != nil {
		t.Fatal(err)
	}
	if gotID != "port:1.2.3.4:22" {
		t.Fatalf("unexpected index id: %q", gotID)
	}
}
