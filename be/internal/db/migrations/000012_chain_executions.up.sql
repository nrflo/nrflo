CREATE TABLE IF NOT EXISTS chain_executions (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    name          TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'canceled')),
    workflow_name TEXT NOT NULL,
    category      TEXT NOT NULL DEFAULT '',
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_chain_exec_project_status ON chain_executions(project_id, status);

CREATE TABLE IF NOT EXISTS chain_execution_items (
    id                    TEXT PRIMARY KEY,
    chain_id              TEXT NOT NULL,
    ticket_id             TEXT NOT NULL,
    position              INTEGER NOT NULL,
    status                TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'skipped', 'canceled')),
    workflow_instance_id  TEXT,
    started_at            TEXT,
    ended_at              TEXT,
    FOREIGN KEY (chain_id) REFERENCES chain_executions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_chain_items_chain_pos ON chain_execution_items(chain_id, position);
CREATE INDEX IF NOT EXISTS idx_chain_items_ticket ON chain_execution_items(chain_id, ticket_id);

CREATE TABLE IF NOT EXISTS chain_execution_locks (
    project_id  TEXT NOT NULL,
    ticket_id   TEXT NOT NULL,
    chain_id    TEXT NOT NULL,
    FOREIGN KEY (chain_id) REFERENCES chain_executions(id) ON DELETE CASCADE,
    UNIQUE(project_id, ticket_id)
);

CREATE INDEX IF NOT EXISTS idx_chain_locks_project_ticket ON chain_execution_locks(project_id, ticket_id);
