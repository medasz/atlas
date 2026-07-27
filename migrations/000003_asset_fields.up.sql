-- 资产字段扩展：host / is_ipv6 / domains 表（对齐 FOFA 字段）
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS is_ipv6 BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE ports ADD COLUMN IF NOT EXISTS host TEXT NOT NULL DEFAULT '';
ALTER TABLE ports ADD COLUMN IF NOT EXISTS is_ipv6 BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS domains (
    name               TEXT PRIMARY KEY,
    registrable_domain TEXT NOT NULL DEFAULT '',
    resolved_ips       TEXT[] NOT NULL DEFAULT '{}',
    cname              TEXT[] NOT NULL DEFAULT '{}',
    org                TEXT NOT NULL DEFAULT '',
    asn                BIGINT NOT NULL DEFAULT 0,
    is_ipv6            BOOLEAN NOT NULL DEFAULT FALSE,
    whois              JSONB,
    first_seen         TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen          TIMESTAMPTZ NOT NULL DEFAULT now()
);
