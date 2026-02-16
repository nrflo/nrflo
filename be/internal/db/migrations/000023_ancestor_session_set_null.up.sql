-- Change ancestor_session_id FK from ON DELETE RESTRICT to ON DELETE SET NULL
-- so that cleanup of old sessions isn't blocked by continuation chains.
PRAGMA foreign_keys = OFF;

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
    FOREIGN KEY (ancestor_session_id) REFERENCES agent_sessions_new(id) ON DELETE SET NULL
);

INSERT INTO agent_sessions_new SELECT * FROM agent_sessions;

DROP TABLE agent_sessions;
ALTER TABLE agent_sessions_new RENAME TO agent_sessions;

-- Recreate indexes
CREATE INDEX IF NOT EXISTS idx_agent_sessions_project_ticket ON agent_sessions(project_id, ticket_id);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_ticket_phase ON agent_sessions(ticket_id, phase);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_wfi ON agent_sessions(workflow_instance_id);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_wfi_status ON agent_sessions(workflow_instance_id, status);

PRAGMA foreign_keys = ON;
