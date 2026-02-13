-- Add callback to agent_sessions status and result CHECK constraints.

PRAGMA foreign_keys = OFF;

-- Rebuild agent_sessions with updated CHECK constraints
CREATE TABLE agent_sessions_new (
    id                    TEXT PRIMARY KEY,
    project_id            TEXT NOT NULL,
    ticket_id             TEXT NOT NULL,
    workflow_instance_id  TEXT NOT NULL,
    phase                 TEXT NOT NULL,
    agent_type            TEXT NOT NULL,
    model_id              TEXT,
    status                TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'completed', 'failed', 'timeout', 'continued', 'project_completed', 'callback')),
    result                TEXT
        CHECK (result IS NULL OR result IN ('pass', 'fail', 'continue', 'timeout', 'callback')),
    result_reason         TEXT,
    pid                   INTEGER,
    findings              TEXT,
    context_left          INTEGER,
    ancestor_session_id   TEXT,
    spawn_command         TEXT,
    prompt_context        TEXT,
    restart_count         INTEGER NOT NULL DEFAULT 0,
    started_at            TEXT,
    ended_at              TEXT,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL,
    FOREIGN KEY (workflow_instance_id) REFERENCES workflow_instances(id) ON DELETE CASCADE,
    FOREIGN KEY (ancestor_session_id) REFERENCES agent_sessions(id) ON DELETE RESTRICT
);

INSERT INTO agent_sessions_new (
    id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id,
    status, result, result_reason, pid, findings, context_left, ancestor_session_id,
    spawn_command, prompt_context, restart_count, started_at, ended_at,
    created_at, updated_at
)
SELECT
    id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id,
    status, result, result_reason, pid, findings, context_left, ancestor_session_id,
    spawn_command, prompt_context, restart_count, started_at, ended_at,
    created_at, updated_at
FROM agent_sessions;

DROP TABLE agent_sessions;
ALTER TABLE agent_sessions_new RENAME TO agent_sessions;

CREATE INDEX idx_agent_sessions_project_ticket ON agent_sessions(project_id, ticket_id);
CREATE INDEX idx_agent_sessions_ticket_phase ON agent_sessions(ticket_id, phase);
CREATE INDEX idx_agent_sessions_wfi ON agent_sessions(workflow_instance_id);
CREATE INDEX idx_agent_sessions_wfi_status ON agent_sessions(workflow_instance_id, status);

PRAGMA foreign_keys = ON;
PRAGMA foreign_key_check;
