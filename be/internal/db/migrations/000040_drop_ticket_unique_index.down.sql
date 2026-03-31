-- Recreate the partial unique index for ticket-scoped workflows (one instance per ticket+workflow).
CREATE UNIQUE INDEX IF NOT EXISTS idx_wfi_ticket_unique
  ON workflow_instances(project_id, ticket_id, workflow_id)
  WHERE scope_type = 'ticket';
