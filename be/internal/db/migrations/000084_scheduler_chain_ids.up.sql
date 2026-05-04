ALTER TABLE scheduled_tasks ADD COLUMN workflow_chain_ids TEXT NOT NULL DEFAULT '[]';
ALTER TABLE schedule_runs ADD COLUMN chain_runs TEXT NOT NULL DEFAULT '[]';
