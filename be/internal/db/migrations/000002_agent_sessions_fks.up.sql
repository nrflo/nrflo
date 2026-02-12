PRAGMA foreign_keys = OFF;

-- 1. Clean orphan sessions whose workflow doesn't exist in workflows table
--    (agent_messages cascade-deletes are inactive with FKs off, so delete manually)
DELETE FROM agent_messages WHERE session_id IN (
    SELECT id FROM agent_sessions
    WHERE (project_id, workflow) NOT IN (SELECT project_id, id FROM workflows)
);
DELETE FROM agent_sessions
WHERE (project_id, workflow) NOT IN (SELECT project_id, id FROM workflows);

-- 2. Null out dangling ancestor references
UPDATE agent_sessions SET ancestor_session_id = NULL
WHERE ancestor_session_id IS NOT NULL
  AND ancestor_session_id NOT IN (SELECT id FROM agent_sessions);

-- 3. Recreate table with FKs
--    Self-reference uses final name "agent_sessions" (works because FKs are off)
CREATE TABLE agent_sessions_new (
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

-- 4. Copy data (explicit columns — drops legacy last_messages & message_stats)
INSERT INTO agent_sessions_new (
    id, project_id, ticket_id, phase, workflow, agent_type, model_id,
    status, context_left, ancestor_session_id, spawn_command, prompt_context,
    created_at, updated_at
)
SELECT
    id, project_id, ticket_id, phase, workflow, agent_type, model_id,
    status, context_left, ancestor_session_id, spawn_command, prompt_context,
    created_at, updated_at
FROM agent_sessions;

-- 5. Drop old table (FKs off, so agent_messages FK won't block)
DROP TABLE agent_sessions;

-- 6. Rename — agent_messages FK resolves to renamed table by name
ALTER TABLE agent_sessions_new RENAME TO agent_sessions;

-- 7. Recreate indexes
CREATE INDEX idx_agent_sessions_project_ticket ON agent_sessions(project_id, ticket_id);
CREATE INDEX idx_agent_sessions_ticket_phase ON agent_sessions(ticket_id, phase);

-- 8. Re-enable and verify
PRAGMA foreign_keys = ON;
PRAGMA foreign_key_check;
