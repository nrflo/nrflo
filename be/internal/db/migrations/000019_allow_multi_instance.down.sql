-- Restore unique constraint. Will fail if duplicate rows exist.
DROP INDEX IF EXISTS idx_wfi_lookup;
DROP INDEX IF EXISTS idx_wfi_ticket_unique;
CREATE UNIQUE INDEX idx_wfi_unique ON workflow_instances(project_id, ticket_id, workflow_id, scope_type);
