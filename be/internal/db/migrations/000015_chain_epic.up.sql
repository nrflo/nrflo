ALTER TABLE chain_executions ADD COLUMN epic_ticket_id TEXT;
CREATE INDEX idx_chain_exec_epic ON chain_executions(project_id, epic_ticket_id);
