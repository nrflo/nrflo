-- Drop 'cli' as a valid execution_mode value.
-- Coerces existing cli rows to cli_interactive, rebuilds both tables with
-- updated CHECK constraints and default, and removes provider_*_modes config rows.
-- SQLite requires table rebuild to modify CHECK constraints.

PRAGMA foreign_keys = OFF;

-- Coerce before rebuilding so INSERT...SELECT does not violate new CHECK.
UPDATE agent_definitions SET execution_mode = 'cli_interactive' WHERE execution_mode = 'cli';
UPDATE system_agent_definitions SET execution_mode = 'cli_interactive' WHERE execution_mode = 'cli';

-- Rebuild agent_definitions: remove cli from CHECK, default cli_interactive.
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
    execution_mode TEXT NOT NULL DEFAULT 'cli_interactive'
        CHECK (execution_mode IN ('cli_interactive', 'api', 'script')),
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

-- Rebuild system_agent_definitions: remove cli from CHECK, default cli_interactive.
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
    execution_mode TEXT NOT NULL DEFAULT 'cli_interactive'
        CHECK (execution_mode IN ('cli_interactive', 'api')),
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

-- Remove provider capability config rows; these are no longer needed.
DELETE FROM config WHERE key IN ('provider_claude_modes', 'provider_codex_modes', 'provider_opencode_modes');

PRAGMA foreign_keys = ON;
