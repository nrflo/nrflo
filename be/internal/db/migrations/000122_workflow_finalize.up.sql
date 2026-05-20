ALTER TABLE workflows ADD COLUMN finalize_success_command TEXT NOT NULL DEFAULT '';
ALTER TABLE workflows ADD COLUMN finalize_success_script_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workflows ADD COLUMN finalize_failure_command TEXT NOT NULL DEFAULT '';
ALTER TABLE workflows ADD COLUMN finalize_failure_script_id TEXT NOT NULL DEFAULT '';
