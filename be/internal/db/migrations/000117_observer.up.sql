-- Extend agent_sessions with kind/observer_scope; relax workflow_instance_id FK to allow
-- observer sessions that are not tied to a workflow instance (NULL = observer).
-- Extend workflows with observer override fields.

PRAGMA foreign_keys = OFF;

CREATE TABLE agent_sessions_new (
    id                    TEXT PRIMARY KEY,
    project_id            TEXT NOT NULL,
    ticket_id             TEXT NOT NULL,
    workflow_instance_id  TEXT,
    phase                 TEXT NOT NULL,
    agent_type            TEXT NOT NULL,
    model_id              TEXT,
    status                TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'completed', 'failed', 'timeout', 'continued', 'project_completed', 'callback', 'user_interactive', 'interactive_completed', 'skipped')),
    result                TEXT
        CHECK (result IS NULL OR result IN ('pass', 'fail', 'continue', 'timeout', 'callback', 'skipped')),
    result_reason         TEXT,
    pid                   INTEGER,
    context_left          INTEGER,
    ancestor_session_id   TEXT,
    spawn_command         TEXT,
    prompt                TEXT,
    system_prompt         TEXT,
    restart_count         INTEGER NOT NULL DEFAULT 0,
    nudge_count           INTEGER NOT NULL DEFAULT 0,
    config                TEXT NOT NULL DEFAULT '',
    started_at            TEXT,
    ended_at              TEXT,
    spawn_token           TEXT,
    effective_mode        TEXT,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL,
    rate_limit_retry_count INTEGER NOT NULL DEFAULT 0,
    rate_limit_until_ts   TEXT,
    last_retry_class      TEXT,
    kind                  TEXT NOT NULL DEFAULT 'workflow_agent',
    observer_scope        TEXT,
    FOREIGN KEY (workflow_instance_id) REFERENCES workflow_instances(id) ON DELETE CASCADE,
    FOREIGN KEY (ancestor_session_id) REFERENCES agent_sessions_new(id) ON DELETE SET NULL
);

INSERT INTO agent_sessions_new (
    id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id,
    status, result, result_reason, pid, context_left, ancestor_session_id,
    spawn_command, prompt, system_prompt, restart_count, nudge_count, config,
    started_at, ended_at, spawn_token, effective_mode, created_at, updated_at,
    rate_limit_retry_count, rate_limit_until_ts, last_retry_class
)
SELECT
    id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id,
    status, result, result_reason, pid, context_left, ancestor_session_id,
    spawn_command, prompt, system_prompt, restart_count, nudge_count, config,
    started_at, ended_at, spawn_token, effective_mode, created_at, updated_at,
    rate_limit_retry_count, rate_limit_until_ts, last_retry_class
FROM agent_sessions;

DROP TABLE agent_sessions;
ALTER TABLE agent_sessions_new RENAME TO agent_sessions;

CREATE INDEX IF NOT EXISTS idx_agent_sessions_project_ticket ON agent_sessions(project_id, ticket_id);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_ticket_phase ON agent_sessions(ticket_id, phase);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_wfi ON agent_sessions(workflow_instance_id);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_wfi_status ON agent_sessions(workflow_instance_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_sessions_spawn_token
    ON agent_sessions(spawn_token) WHERE spawn_token IS NOT NULL;

PRAGMA foreign_keys = ON;

ALTER TABLE workflows ADD COLUMN observer_context TEXT NOT NULL DEFAULT '';
ALTER TABLE workflows ADD COLUMN observer_provider TEXT DEFAULT NULL;
ALTER TABLE workflows ADD COLUMN observer_model TEXT DEFAULT NULL;
