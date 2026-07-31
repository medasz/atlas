-- Atlas 基础 Schema（存储/任务/漏洞/审计/配置/模版/黑名单/IP存活/资产变更履历）
-- 资产本体（hosts/ports/domains）已全面保存于 Elasticsearch，PG 专注于关系型业务数据

-- 1. IP 存活与生命周期状态表（主机打卡）
CREATE TABLE IF NOT EXISTS ip_survivals (
    ip          TEXT PRIMARY KEY,
    open_ports  INT NOT NULL DEFAULT 0,
    is_ipv6     BOOLEAN NOT NULL DEFAULT false,
    first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ip_survivals_last_seen ON ip_survivals(last_seen);

-- 2. 资产变更历史事件表（端口开放/下线与服务变更）
CREATE TABLE IF NOT EXISTS asset_history (
    id           BIGSERIAL PRIMARY KEY,
    ip           TEXT NOT NULL,
    port         INT NOT NULL,
    proto        TEXT NOT NULL DEFAULT 'tcp',
    change_type  TEXT NOT NULL,  -- 'port_opened' | 'port_closed' | 'service_changed'
    old_service  JSONB,
    new_service  JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_asset_history_ip_port ON asset_history(ip, port);
CREATE INDEX IF NOT EXISTS idx_asset_history_created_at ON asset_history(created_at);

-- 3. 黑名单（不扫描资产）
CREATE TABLE IF NOT EXISTS blacklist (
    id          BIGSERIAL PRIMARY KEY,
    type        TEXT NOT NULL,   -- ip | cidr | domain
    value       TEXT NOT NULL,
    operator    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (type, value)
);

-- 4. 任务
CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL,   -- scan | vuln
    scope       JSONB NOT NULL,
    schedule    JSONB,
    rate_limit  JSONB,
    status      INT NOT NULL DEFAULT 0,   -- 0 pending 1 running 2 done 3 paused 4 failed
    progress    JSONB,           -- {total:int, done:int}
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);

-- 5. 任务子项（断点续扫）
CREATE TABLE IF NOT EXISTS task_items (
    id          BIGSERIAL PRIMARY KEY,
    task_id     TEXT NOT NULL,
    target      TEXT NOT NULL,
    ports       TEXT NOT NULL DEFAULT '',
    status      INT NOT NULL DEFAULT 0,   -- 0 pending 1 processing 2 done 3 filtered 4 failed
    result      JSONB,
    UNIQUE (task_id, target, ports)
);
CREATE INDEX IF NOT EXISTS idx_items_task_status ON task_items(task_id, status);

-- 6. 漏洞结果
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

-- 7. 审计日志（可开关）
CREATE TABLE IF NOT EXISTS audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    operator    TEXT,
    time        TIMESTAMPTZ NOT NULL DEFAULT now(),
    target      TEXT,
    task_id     TEXT,
    action      TEXT
);

-- 8. 检测模板（nuclei 风格）
CREATE TABLE IF NOT EXISTS templates (
    id        TEXT PRIMARY KEY,
    kind      TEXT NOT NULL,   -- yaml | go
    content   TEXT NOT NULL,
    enabled   BOOLEAN NOT NULL DEFAULT true
);

-- 9. 配置
CREATE TABLE IF NOT EXISTS config (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
