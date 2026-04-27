-- API-mode foundation: extend agent_definitions and add tool/credential tables.

ALTER TABLE agent_definitions ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'cli'
    CHECK (execution_mode IN ('cli', 'api'));
ALTER TABLE agent_definitions ADD COLUMN tools TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_definitions ADD COLUMN api_max_iterations INTEGER;

CREATE TABLE tool_definitions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    input_schema TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    auth_method TEXT NOT NULL DEFAULT 'none'
        CHECK (auth_method IN ('none', 'bearer_env', 'bearer_secret_ref')),
    auth_ref TEXT,
    timeout_sec INTEGER NOT NULL DEFAULT 30,
    project_id TEXT,
    workflow_id TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE api_credentials (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    project_id TEXT,
    secret_ref TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_api_credentials_provider_project ON api_credentials (provider, project_id);
CREATE INDEX idx_tool_definitions_project ON tool_definitions (project_id);
CREATE INDEX idx_tool_definitions_workflow ON tool_definitions (workflow_id);
