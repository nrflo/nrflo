-- Extend agent_definitions and system_agent_definitions execution_mode CHECK constraints
-- to include 'cli_interactive'. Also deletes the project-level interactive_cli_mode config
-- toggle rows, which are superseded by per-agent execution_mode='cli_interactive'.
-- SQLite requires table rebuild to modify CHECK constraints.

PRAGMA foreign_keys = OFF;

-- Rebuild agent_definitions with cli_interactive in the CHECK list.
CREATE TABLE agent_definitions_new (
    id          TEXT NOT NULL,
    project_id  TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    model       TEXT NOT NULL DEFAULT 'sonnet',
    timeout     INTEGER NOT NULL DEFAULT 20,
    prompt      TEXT NOT NULL DEFAULT '',
    restart_threshold INTEGER,
    max_fail_restarts INTEGER,
    stall_start_timeout_sec INTEGER,
    stall_running_timeout_sec INTEGER,
    tag         TEXT NOT NULL DEFAULT '',
    low_consumption_model TEXT NOT NULL DEFAULT '',
    layer       INTEGER NOT NULL DEFAULT 0,
    execution_mode TEXT NOT NULL DEFAULT 'cli'
        CHECK (execution_mode IN ('cli', 'cli_interactive', 'api', 'script')),
    tools       TEXT NOT NULL DEFAULT '',
    api_max_iterations INTEGER,
    python_script_id TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (project_id, workflow_id, id),
    FOREIGN KEY (project_id, workflow_id) REFERENCES workflows(project_id, id) ON DELETE CASCADE
);

INSERT INTO agent_definitions_new
    SELECT id, project_id, workflow_id, model, timeout, prompt,
           restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec,
           tag, low_consumption_model, layer, execution_mode, tools, api_max_iterations,
           python_script_id, created_at, updated_at
    FROM agent_definitions;

DROP TABLE agent_definitions;
ALTER TABLE agent_definitions_new RENAME TO agent_definitions;

-- Rebuild system_agent_definitions with cli_interactive in the CHECK list.
-- script remains invalid for system agents.
CREATE TABLE system_agent_definitions_new (
    id          TEXT PRIMARY KEY,
    model       TEXT NOT NULL DEFAULT 'sonnet',
    timeout     INTEGER NOT NULL DEFAULT 20,
    prompt      TEXT NOT NULL DEFAULT '',
    restart_threshold INTEGER,
    max_fail_restarts INTEGER,
    stall_start_timeout_sec INTEGER,
    stall_running_timeout_sec INTEGER,
    role        TEXT NOT NULL DEFAULT '',
    execution_mode TEXT NOT NULL DEFAULT 'cli'
        CHECK (execution_mode IN ('cli', 'cli_interactive', 'api')),
    tools       TEXT NOT NULL DEFAULT '',
    api_max_iterations INTEGER,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

INSERT INTO system_agent_definitions_new
    SELECT id, model, timeout, prompt,
           restart_threshold, max_fail_restarts, stall_start_timeout_sec, stall_running_timeout_sec,
           role, execution_mode, tools, api_max_iterations,
           created_at, updated_at
    FROM system_agent_definitions;

DROP TABLE system_agent_definitions;
ALTER TABLE system_agent_definitions_new RENAME TO system_agent_definitions;

-- Recreate the unique index dropped with the table.
CREATE UNIQUE INDEX idx_system_agent_role_mode ON system_agent_definitions (role, execution_mode);

-- Delete per-project interactive_cli_mode config rows; the feature is now driven
-- by per-agent execution_mode='cli_interactive' instead of a project-wide toggle.
DELETE FROM config WHERE key = 'interactive_cli_mode';

PRAGMA foreign_keys = ON;
