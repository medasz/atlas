-- 任务子项支持端口块粒度（进度可见性细化到端口块）
ALTER TABLE task_items ADD COLUMN IF NOT EXISTS ports TEXT NOT NULL DEFAULT '';
ALTER TABLE task_items DROP CONSTRAINT IF EXISTS task_items_task_id_target_key;
ALTER TABLE task_items
  ADD CONSTRAINT task_items_task_id_target_ports_key UNIQUE (task_id, target, ports);
