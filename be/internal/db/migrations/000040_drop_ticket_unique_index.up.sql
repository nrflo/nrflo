-- Drop the ticket-scoped unique index to allow multiple instances per ticket+workflow.
-- This aligns ticket workflows with the project-scoped workflow model.
DROP INDEX IF EXISTS idx_wfi_ticket_unique;
