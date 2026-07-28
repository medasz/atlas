-- 配置表：每行一个配置段（scan/audit/auth/http），value 为该段的 JSON 序列化
CREATE TABLE IF NOT EXISTS config (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
-- 兼容已存在但缺少 updated_at 列的旧表（升级 / 镜像未重建场景）：
-- 仅建表时不带 updated_at，再用幂等 ALTER 补列，避免 IF NOT EXISTS 跳过整表导致缺列。
ALTER TABLE config ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
