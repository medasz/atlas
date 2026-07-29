-- 补全 domains 表字段：原 000003 使用 CREATE TABLE IF NOT EXISTS，
-- 在 domains 已由 000001 建表时为 no-op，导致 org/asn/is_ipv6 列缺失。
-- 此处用 ALTER 补列，保证 ListAllDomains（ReindexFromPG）在真实库上可运行。
ALTER TABLE domains ADD COLUMN IF NOT EXISTS org TEXT NOT NULL DEFAULT '';
ALTER TABLE domains ADD COLUMN IF NOT EXISTS asn BIGINT NOT NULL DEFAULT 0;
ALTER TABLE domains ADD COLUMN IF NOT EXISTS is_ipv6 BOOLEAN NOT NULL DEFAULT FALSE;
