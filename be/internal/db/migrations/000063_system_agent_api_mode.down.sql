DROP INDEX IF EXISTS idx_system_agent_role_mode;
DELETE FROM system_agent_definitions WHERE id = 'context-saver-api';
ALTER TABLE system_agent_definitions DROP COLUMN api_max_iterations;
ALTER TABLE system_agent_definitions DROP COLUMN tools;
ALTER TABLE system_agent_definitions DROP COLUMN execution_mode;
ALTER TABLE system_agent_definitions DROP COLUMN role;
