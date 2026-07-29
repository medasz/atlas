package esasset

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"atlas/internal/assetstore"
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

// TestGetHost 验证 GetHost 命中返回 model.Asset；未命中返回 ErrNotFound
func TestGetHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "_doc/host:1.2.3.4") {
			w.Write([]byte(`{"found":true,"_source":{"doc_type":"host","ip":"1.2.3.4","org":"acme","os":"Linux"}}`))
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
