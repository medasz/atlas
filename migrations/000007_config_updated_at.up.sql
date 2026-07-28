-- 修复：000006 已 applied 但当时建表语句未含 updated_at 的旧库。
-- 无论是否重复执行，ADD COLUMN IF NOT EXISTS 均安全幂等。
ALTER TABLE config ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
