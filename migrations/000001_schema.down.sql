-- Atlas 初始 schema 回滚（与 000001_schema.up.sql 对应）

DROP TABLE IF EXISTS config;
DROP TABLE IF EXISTS templates;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS vulns;
DROP TABLE IF EXISTS task_items;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS blacklist;
DROP TABLE IF EXISTS asset_history;
DROP TABLE IF EXISTS ip_survivals;
