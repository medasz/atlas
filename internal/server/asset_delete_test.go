package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"atlas/internal/config"
	"atlas/internal/model"
	"atlas/internal/store"
)

type deleteAssetStore struct {
	deleted     model.Asset
	deletedHost string
	ports       []model.Asset
}

func (s *deleteAssetStore) Upsert(context.Context, model.Asset) error { return nil }
func (s *deleteAssetStore) Delete(_ context.Context, asset model.Asset) error {
	s.deleted = asset
	return nil
}
func (s *deleteAssetStore) DeleteHost(_ context.Context, ip string) (int64, error) {
	s.deletedHost = ip
	return 2, nil
}
func (s *deleteAssetStore) GetHost(context.Context, string) (model.Asset, error) {
	return model.Asset{}, nil
}
func (s *deleteAssetStore) ListPortsByIP(context.Context, string) ([]model.Asset, error) {
	return s.ports, nil
}
func (s *deleteAssetStore) ListDomains(context.Context) ([]model.Asset, error) { return nil, nil }
func (s *deleteAssetStore) GetHostDetail(context.Context, string) (model.Asset, []model.Asset, error) {
	return model.Asset{}, nil, nil
}
func (s *deleteAssetStore) SearchAssets(context.Context, string, bool, int, int) (*store.SearchResult, error) {
	return &store.SearchResult{}, nil
}

func newDeleteAssetServer(asset *deleteAssetStore) *Server {
	return New(Deps{Cfg: &config.Config{Auth: config.AuthConfig{Enabled: false}}, Asset: asset})
}

func TestDeletePortAsset(t *testing.T) {
	asset := &deleteAssetStore{}
	req := httptest.NewRequest(http.MethodDelete, "/api/assets?ip=192.168.30.34&port=443", nil)
	res := httptest.NewRecorder()
	newDeleteAssetServer(asset).Engine().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
	if asset.deleted.IP != "192.168.30.34" || asset.deleted.Port != 443 {
		t.Fatalf("unexpected deleted asset: %+v", asset.deleted)
	}
}

func TestDeleteDomainAsset(t *testing.T) {
	asset := &deleteAssetStore{}
	req := httptest.NewRequest(http.MethodDelete, "/api/assets?domain=app.example.com", nil)
	res := httptest.NewRecorder()
	newDeleteAssetServer(asset).Engine().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
	if asset.deleted.Domain != "app.example.com" {
		t.Fatalf("unexpected deleted asset: %+v", asset.deleted)
	}
}

func TestDeleteDomainAssetRejectsUnsafeTarget(t *testing.T) {
	for _, domain := range []string{
		"app/example.com",
		"app?example.com",
		"app#example.com",
		"app%example.com",
		"app\\example.com",
		"app example.com",
		"app\nexample.com",
	} {
		t.Run(url.QueryEscape(domain), func(t *testing.T) {
			asset := &deleteAssetStore{}
			req := httptest.NewRequest(http.MethodDelete, "/api/assets?domain="+url.QueryEscape(domain), nil)
			res := httptest.NewRecorder()
			newDeleteAssetServer(asset).Engine().ServeHTTP(res, req)

			if res.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %q, got %d: %s", domain, res.Code, res.Body.String())
			}
			if asset.deleted.IP != "" || asset.deleted.Port != 0 || asset.deleted.Domain != "" {
				t.Fatalf("unsafe domain reached asset store: %+v", asset.deleted)
			}
		})
	}
}

func TestDeleteHostAssets(t *testing.T) {
	asset := &deleteAssetStore{}
	req := httptest.NewRequest(http.MethodDelete, "/api/hosts/192.168.30.34", nil)
	res := httptest.NewRecorder()
	newDeleteAssetServer(asset).Engine().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
	if asset.deletedHost != "192.168.30.34" {
		t.Fatalf("unexpected deleted host: %q", asset.deletedHost)
	}
}

func TestDeleteAssetRejectsAmbiguousTarget(t *testing.T) {
	asset := &deleteAssetStore{}
	req := httptest.NewRequest(http.MethodDelete, "/api/assets?ip=192.168.30.34", nil)
	res := httptest.NewRecorder()
	newDeleteAssetServer(asset).Engine().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", res.Code, res.Body.String())
	}
}
