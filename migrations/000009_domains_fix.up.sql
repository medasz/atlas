-- 补全 domains 表字段：原 000003 使用 CREATE TABLE IF NOT EXISTS，
-- 在 domains 已由 000001 建表时为 no-op，导致 org/asn/is_ipv6 列缺失。
-- 此处用 ALTER 补列，保证（若 domains 表尚存）字段完整。
-- 用 DO 块守卫：表已不存在（如已被删表迁移清理 / 本迁移被重放）时安全跳过，
-- 避免 ALTER 不存在的表导致迁移失败（SQLSTATE 42P01）。
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'domains'
  ) THEN
    ALTER TABLE domains ADD COLUMN IF NOT EXISTS org TEXT NOT NULL DEFAULT '';
    ALTER TABLE domains ADD COLUMN IF NOT EXISTS asn BIGINT NOT NULL DEFAULT 0;
    ALTER TABLE domains ADD COLUMN IF NOT EXISTS is_ipv6 BOOLEAN NOT NULL DEFAULT FALSE;
  END IF;
END
$$;
