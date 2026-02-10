CREATE TABLE IF NOT EXISTS agent_definitions (
    id          TEXT NOT NULL,
    project_id  TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    model       TEXT NOT NULL DEFAULT 'sonnet',
    timeout     INTEGER NOT NULL DEFAULT 20,
    prompt      TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (project_id, workflow_id, id),
    FOREIGN KEY (project_id, workflow_id) REFERENCES workflows(project_id, id) ON DELETE CASCADE
);
