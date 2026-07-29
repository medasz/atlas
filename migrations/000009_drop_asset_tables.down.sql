-- 灾难恢复用：重建最小资产表（数据已迁 ES，此回退仅恢复结构）
CREATE TABLE IF NOT EXISTS hosts (
  ip TEXT PRIMARY KEY, asn INT, org TEXT, geo JSONB, os TEXT,
  is_ipv6 BOOLEAN, open_ports INT, first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS ports (
  ip TEXT, port INT, proto TEXT, state TEXT, service TEXT, version TEXT,
  banner TEXT, cert JSONB, title TEXT, host TEXT, is_ipv6 BOOLEAN, webinfo JSONB,
  first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ,
  PRIMARY KEY (ip, port, proto)
);
CREATE TABLE IF NOT EXISTS domains (
  name TEXT PRIMARY KEY, registrable_domain TEXT, resolved_ips TEXT,
  cname TEXT, org TEXT, asn INT, is_ipv6 BOOLEAN, whois JSONB,
  first_seen TIMESTAMPTZ, last_seen TIMESTAMPTZ
);
