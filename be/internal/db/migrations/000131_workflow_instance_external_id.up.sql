ALTER TABLE workflow_instances ADD COLUMN external_id TEXT;
ALTER TABLE workflow_instances ADD COLUMN external_context TEXT;
CREATE INDEX IF NOT EXISTS idx_workflow_instances_external_id ON workflow_instances(external_id);
