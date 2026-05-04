-- SQLite does not support DROP COLUMN in older versions; recreate without the column.
CREATE TABLE workflow_chain_run_steps_old AS SELECT id, chain_run_id, position, workflow_name, scope_type, workflow_instance_id, ticket_id, instructions_used, status, started_at, ended_at, created_at, updated_at FROM workflow_chain_run_steps;
DROP TABLE workflow_chain_run_steps;
ALTER TABLE workflow_chain_run_steps_old RENAME TO workflow_chain_run_steps;
