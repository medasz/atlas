package assetstore

import (
	"context"
	"errors"

	"atlas/internal/model"
	"atlas/internal/store"
)

// HostAggregate is the lightweight payload used by the IP aggregation page.
// It intentionally excludes per-port fingerprint fields such as banners.
type HostAggregate struct {
	Host        model.Asset      `json:"host"`
	Total       int64            `json:"total"`
	StateCounts map[string]int64 `json:"state_counts"`
}

// PortPage contains only the current page of ports plus enough metadata for
// pagination and state filtering.
type PortPage struct {
	Items       []model.Asset    `json:"items"`
	Total       int64            `json:"total"`
	StateCounts map[string]int64 `json:"state_counts"`
}

// ErrNotFound 资产不存在（ES 文档 404 映射）
var ErrNotFound = errors.New("asset not found")

// AssetStore 资产本体的统一存储接口（基于 model.Asset）；实现可为 ES（本期唯一实现）。
type AssetStore interface {
	Upsert(ctx context.Context, a model.Asset) error
	Delete(ctx context.Context, a model.Asset) error
	DeleteHost(ctx context.Context, ip string) (int64, error)
	GetHost(ctx context.Context, ip string) (model.Asset, error) // 未找到返回 ErrNotFound
	ListPortsByIP(ctx context.Context, ip string) ([]model.Asset, error)
	GetHostAggregate(ctx context.Context, ip string) (HostAggregate, error)
	ListPortPage(ctx context.Context, ip, state, sort string, page, pageSize int) (PortPage, error)
	GetPort(ctx context.Context, ip string, port int) (model.Asset, error)
	ListDomains(ctx context.Context) ([]model.Asset, error)
	GetHostDetail(ctx context.Context, ip string) (model.Asset, []model.Asset, error)
	SearchAssets(ctx context.Context, q string, aggregated bool, page, pageSize int) (*store.SearchResult, error)
}
