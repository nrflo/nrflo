ALTER TABLE workflows ADD COLUMN pause_event_command TEXT NOT NULL DEFAULT '';
ALTER TABLE workflows ADD COLUMN pause_event_script_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workflow_layer_policies ADD COLUMN pause_after INTEGER NOT NULL DEFAULT 0;
