package assetstore

import (
	"context"
	"errors"

	"atlas/internal/model"
	"atlas/internal/store"
)

// ErrNotFound 资产不存在（ES 文档 404 映射）
var ErrNotFound = errors.New("asset not found")

// AssetStore 资产本体的统一存储接口（基于 model.Asset）；实现可为 ES（本期唯一实现）。
type AssetStore interface {
	Upsert(ctx context.Context, a model.Asset) error
	Delete(ctx context.Context, a model.Asset) error
	DeleteHost(ctx context.Context, ip string) (int64, error)
	GetHost(ctx context.Context, ip string) (model.Asset, error) // 未找到返回 ErrNotFound
	ListPortsByIP(ctx context.Context, ip string) ([]model.Asset, error)
	ListDomains(ctx context.Context) ([]model.Asset, error)
	GetHostDetail(ctx context.Context, ip string) (model.Asset, []model.Asset, error)
	SearchAssets(ctx context.Context, q string, aggregated bool, page, pageSize int) (*store.SearchResult, error)
}
