-- Atlas 初始 schema（资产 / 任务 / 漏洞 / 审计 / 模板 / 配置）
-- 执行顺序由文件名字典序保证；已应用记录写入 schema_migrations 去重。

-- 主机资产
CREATE TABLE IF NOT EXISTS hosts (
  ip          TEXT PRIMARY KEY,
  asn         INT,
  org         TEXT,
  geo         JSONB,
  os          TEXT,
  open_ports  INTEGER[],
  first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_hosts_last_seen ON hosts(last_seen);

-- 端口 / 服务
CREATE TABLE IF NOT EXISTS ports (
  ip          TEXT NOT NULL,
  port        INT  NOT NULL,
  proto       TEXT NOT NULL DEFAULT 'tcp',
  service     TEXT,
  version     TEXT,
  banner      TEXT,
  cert        JSONB,
  title       TEXT,
  webinfo     JSONB,
  first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (ip, port, proto)
);
CREATE INDEX IF NOT EXISTS idx_ports_service ON ports(service);

-- 域名（二期被动源使用，先建表留空）
CREATE TABLE IF NOT EXISTS domains (
  name                TEXT PRIMARY KEY,
  registrable_domain  TEXT,
  resolved_ips        TEXT[],
  cname               TEXT[],
  whois               JSONB,
  cert                JSONB,
  first_seen          TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 资产历史快照（断点 / 变更追踪）
CREATE TABLE IF NOT EXISTS asset_history (
  id          BIGSERIAL PRIMARY KEY,
  entity_type TEXT,
  entity_key  TEXT,
  del_time    TIMESTAMPTZ,
  type        TEXT,   -- update
  snapshot    JSONB
);
CREATE INDEX IF NOT EXISTS idx_asset_hist_key ON asset_history(entity_type, entity_key);

-- 黑名单（不扫描资产）
CREATE TABLE IF NOT EXISTS blacklist (
  id          BIGSERIAL PRIMARY KEY,
  type        TEXT NOT NULL,   -- ip | cidr | domain
  value       TEXT NOT NULL,
  operator    TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (type, value)
);

-- 任务
CREATE TABLE IF NOT EXISTS tasks (
  id          TEXT PRIMARY KEY,
  kind        TEXT NOT NULL,   -- scan | vuln
  scope       JSONB NOT NULL,
  schedule    JSONB,
  rate_limit  JSONB,
  status      INT NOT NULL DEFAULT 0,   -- 0 pending 1 running 2 done
  progress    JSONB,           -- {total:int, done:int}
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);

-- 任务子项（断点续扫）
CREATE TABLE IF NOT EXISTS task_items (
  id          BIGSERIAL PRIMARY KEY,
  task_id     TEXT NOT NULL,
  target      TEXT NOT NULL,
  status      INT NOT NULL DEFAULT 0,   -- 0 pending 1 done 2 filtered
  result      JSONB,
  UNIQUE (task_id, target)
);
CREATE INDEX IF NOT EXISTS idx_items_task_status ON task_items(task_id, status);

-- 漏洞结果
CREATE TABLE IF NOT EXISTS vulns (
  id            BIGSERIAL PRIMARY KEY,
  asset_ref     TEXT NOT NULL,
  kpid          TEXT,
  cve           TEXT,
  name          TEXT,
  level         INT,
  type          TEXT,
  proof         TEXT,
  status        TEXT NOT NULL DEFAULT 'open',  -- open|fixed|recur
  first_found   TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_verified TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (asset_ref, kpid)
);
CREATE INDEX IF NOT EXISTS idx_vulns_status ON vulns(status);

-- 审计日志（可开关）
CREATE TABLE IF NOT EXISTS audit_logs (
  id          BIGSERIAL PRIMARY KEY,
  operator    TEXT,
  time        TIMESTAMPTZ NOT NULL DEFAULT now(),
  target      TEXT,
  task_id     TEXT,
  action      TEXT
);

-- 检测模板（nuclei 风格）
CREATE TABLE IF NOT EXISTS templates (
  id        TEXT PRIMARY KEY,
  kind      TEXT NOT NULL,   -- yaml | go
  content   TEXT NOT NULL,
  enabled   BOOLEAN NOT NULL DEFAULT true
);

-- 配置
CREATE TABLE IF NOT EXISTS config (
  key     TEXT PRIMARY KEY,
  value   JSONB NOT NULL
);
