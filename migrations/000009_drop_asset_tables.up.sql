-- 资产本体已全面迁移至 Elasticsearch（统一 model.Asset，方案 B）。
-- 删除 PG 资产表；资产读写只经 AssetStore → ES。
DROP TABLE IF EXISTS ports;
DROP TABLE IF EXISTS hosts;
DROP TABLE IF EXISTS domains;
