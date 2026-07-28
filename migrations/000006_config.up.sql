-- 配置表：每行一个配置段（scan/audit/auth/http），value 为该段的 JSON 序列化
CREATE TABLE IF NOT EXISTS config (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
