PRAGMA foreign_keys = OFF;

-- 1. Create workflow_instances table
CREATE TABLE IF NOT EXISTS workflow_instances (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    ticket_id       TEXT NOT NULL,
    workflow_id     TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'completed', 'failed')),
    category        TEXT,
    current_phase   TEXT,
    phase_order     TEXT NOT NULL,
    phases          TEXT NOT NULL DEFAULT '{}',
    findings        TEXT NOT NULL DEFAULT '{}',
    retry_count     INTEGER NOT NULL DEFAULT 0,
    parent_session  TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    FOREIGN KEY (project_id, workflow_id) REFERENCES workflows(project_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (project_id, ticket_id) REFERENCES tickets(project_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_wfi_unique ON workflow_instances(project_id, ticket_id, workflow_id);
CREATE INDEX IF NOT EXISTS idx_wfi_ticket ON workflow_instances(project_id, ticket_id);

-- 2. Recreate agent_sessions with workflow_instance_id instead of workflow FK
--    Delete existing agent_messages first (cascade is inactive with FKs off)
DELETE FROM agent_messages WHERE session_id IN (SELECT id FROM agent_sessions);
DELETE FROM agent_sessions;

CREATE TABLE agent_sessions_new (
    id                    TEXT PRIMARY KEY,
    project_id            TEXT NOT NULL,
    ticket_id             TEXT NOT NULL,
    workflow_instance_id  TEXT NOT NULL,
    phase                 TEXT NOT NULL,
    agent_type            TEXT NOT NULL,
    model_id              TEXT,
    status                TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'completed', 'failed', 'timeout', 'continued')),
    result                TEXT
        CHECK (result IS NULL OR result IN ('pass', 'fail', 'continue', 'timeout')),
    result_reason         TEXT,
    pid                   INTEGER,
    findings              TEXT,
    context_left          INTEGER,
    ancestor_session_id   TEXT,
    spawn_command         TEXT,
    prompt_context        TEXT,
    started_at            TEXT,
    ended_at              TEXT,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL,
    FOREIGN KEY (workflow_instance_id) REFERENCES workflow_instances(id) ON DELETE CASCADE,
    FOREIGN KEY (ancestor_session_id) REFERENCES agent_sessions(id) ON DELETE RESTRICT
);

DROP TABLE agent_sessions;
ALTER TABLE agent_sessions_new RENAME TO agent_sessions;

CREATE INDEX idx_agent_sessions_project_ticket ON agent_sessions(project_id, ticket_id);
CREATE INDEX idx_agent_sessions_ticket_phase ON agent_sessions(ticket_id, phase);
CREATE INDEX idx_agent_sessions_wfi ON agent_sessions(workflow_instance_id);
CREATE INDEX idx_agent_sessions_wfi_status ON agent_sessions(workflow_instance_id, status);

PRAGMA foreign_keys = ON;
PRAGMA foreign_key_check;
