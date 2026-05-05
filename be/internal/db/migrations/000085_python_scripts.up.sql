-- Create python_scripts table for project-scoped Python scripts stored in DB.
CREATE TABLE python_scripts (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    code        TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE UNIQUE INDEX python_scripts_project_id_id ON python_scripts(project_id, id);

-- Extend agent_definitions execution_mode CHECK to include 'script' and add python_script_id column.
-- SQLite requires table rebuild to modify CHECK constraints.
PRAGMA foreign_keys = OFF;

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
        CHECK (execution_mode IN ('cli', 'api', 'script')),
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
           NULL, created_at, updated_at
    FROM agent_definitions;

DROP TABLE agent_definitions;
ALTER TABLE agent_definitions_new RENAME TO agent_definitions;

PRAGMA foreign_keys = ON;
