-- 资产检索分页性能索引：加速 UNION 检索与 count(*) 的常见过滤列
-- 注意：ports 表不含 org/asn/is_ipv6 列（这些属 hosts），故仅对存在的列建索引
CREATE INDEX IF NOT EXISTS idx_ports_ip ON ports(ip);
CREATE INDEX IF NOT EXISTS idx_hosts_org ON hosts(org);
CREATE INDEX IF NOT EXISTS idx_domains_registrable_domain ON domains(registrable_domain);
CREATE INDEX IF NOT EXISTS idx_domains_name ON domains(name);
