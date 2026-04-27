DROP INDEX IF EXISTS idx_tool_definitions_workflow;
DROP INDEX IF EXISTS idx_tool_definitions_project;
DROP INDEX IF EXISTS idx_api_credentials_provider_project;
DROP TABLE IF EXISTS api_credentials;
DROP TABLE IF EXISTS tool_definitions;

ALTER TABLE agent_definitions DROP COLUMN api_max_iterations;
ALTER TABLE agent_definitions DROP COLUMN tools;
ALTER TABLE agent_definitions DROP COLUMN execution_mode;
