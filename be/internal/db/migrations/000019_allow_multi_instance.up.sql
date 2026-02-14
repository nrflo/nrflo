-- Drop single-instance-per-project-workflow unique constraint.
-- Multiple concurrent instances of the same project workflow are now allowed.
DROP INDEX IF EXISTS idx_wfi_unique;

-- Non-unique lookup index for query performance
CREATE INDEX IF NOT EXISTS idx_wfi_lookup ON workflow_instances(project_id, ticket_id, workflow_id, scope_type);

-- Keep uniqueness for ticket-scoped workflows (one instance per ticket+workflow)
CREATE UNIQUE INDEX IF NOT EXISTS idx_wfi_ticket_unique
  ON workflow_instances(project_id, ticket_id, workflow_id)
  WHERE scope_type = 'ticket';
