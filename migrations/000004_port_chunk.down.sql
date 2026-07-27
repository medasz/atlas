ALTER TABLE task_items DROP CONSTRAINT IF EXISTS task_items_task_id_target_ports_key;
ALTER TABLE task_items DROP COLUMN IF EXISTS ports;
