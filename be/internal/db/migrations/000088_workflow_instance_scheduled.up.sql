ALTER TABLE workflow_instances ADD COLUMN scheduled_task_id TEXT REFERENCES scheduled_tasks(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_workflow_instances_scheduled ON workflow_instances(scheduled_task_id);
