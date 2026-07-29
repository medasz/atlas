-- 灾难恢复用：回退 000008_domains_fix 补列（Task 10 删表迁移会整体删除 domains）
ALTER TABLE domains DROP COLUMN IF EXISTS org;
ALTER TABLE domains DROP COLUMN IF EXISTS asn;
ALTER TABLE domains DROP COLUMN IF EXISTS is_ipv6;
