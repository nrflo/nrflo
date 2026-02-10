PRAGMA foreign_keys = OFF;

-- 1. Delete agent_messages for sessions being recreated
DELETE FROM agent_messages WHERE session_id IN (SELECT id FROM agent_sessions);
DELETE FROM agent_sessions;

-- 2. Recreate agent_sessions with old schema (workflow TEXT FK)
CREATE TABLE agent_sessions_old (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    ticket_id TEXT NOT NULL,
    phase TEXT NOT NULL,
    workflow TEXT NOT NULL,
    agent_type TEXT NOT NULL,
    model_id TEXT,
    status TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'completed', 'failed', 'timeout', 'continued')),
    context_left INTEGER,
    ancestor_session_id TEXT,
    spawn_command TEXT,
    prompt_context TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (project_id, workflow) REFERENCES workflows(project_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (ancestor_session_id) REFERENCES agent_sessions(id) ON DELETE RESTRICT
);

DROP TABLE agent_sessions;
ALTER TABLE agent_sessions_old RENAME TO agent_sessions;

CREATE INDEX idx_agent_sessions_project_ticket ON agent_sessions(project_id, ticket_id);
CREATE INDEX idx_agent_sessions_ticket_phase ON agent_sessions(ticket_id, phase);

-- 3. Drop workflow_instances
DROP TABLE IF EXISTS workflow_instances;

PRAGMA foreign_keys = ON;
