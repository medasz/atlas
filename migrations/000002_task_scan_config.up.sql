-- Development data reset: legacy tasks have no immutable scan configuration.
-- Assets, configuration, audit logs, vulnerabilities, and blacklists are retained.
DELETE FROM task_items;
DELETE FROM tasks;

ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS scan_config JSONB NOT NULL DEFAULT '{}'::jsonb;
