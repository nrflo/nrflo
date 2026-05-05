DROP INDEX IF EXISTS idx_workflow_instances_scheduled;
ALTER TABLE workflow_instances DROP COLUMN scheduled_task_id;
