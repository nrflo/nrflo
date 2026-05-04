CREATE TABLE workflow_chains (
    id          TEXT NOT NULL,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (project_id, id)
);

CREATE TABLE workflow_chain_steps (
    id                     TEXT PRIMARY KEY,
    project_id             TEXT NOT NULL,
    chain_id               TEXT NOT NULL,
    position               INTEGER NOT NULL,
    workflow_name          TEXT NOT NULL,
    scope_type             TEXT NOT NULL CHECK(scope_type IN ('project', 'ticket')),
    base_instructions      TEXT NOT NULL DEFAULT '',
    require_ticket_handoff INTEGER NOT NULL DEFAULT 0,
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL,
    FOREIGN KEY (project_id, chain_id) REFERENCES workflow_chains(project_id, id) ON DELETE CASCADE,
    UNIQUE (chain_id, position)
);

CREATE INDEX IF NOT EXISTS idx_workflow_chain_steps_chain ON workflow_chain_steps (chain_id, position);

CREATE TABLE workflow_chain_runs (
    id                   TEXT PRIMARY KEY,
    project_id           TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    chain_id             TEXT NOT NULL,
    status               TEXT NOT NULL CHECK(status IN ('pending', 'running', 'completed', 'failed', 'canceled')),
    initial_instructions TEXT NOT NULL DEFAULT '',
    triggered_by         TEXT NOT NULL DEFAULT '',
    current_position     INTEGER NOT NULL DEFAULT 0,
    started_at           TEXT,
    completed_at         TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    FOREIGN KEY (project_id, chain_id) REFERENCES workflow_chains(project_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_workflow_chain_runs_status ON workflow_chain_runs (project_id, status);

CREATE TABLE workflow_chain_run_steps (
    id                   TEXT PRIMARY KEY,
    chain_run_id         TEXT NOT NULL REFERENCES workflow_chain_runs(id) ON DELETE CASCADE,
    position             INTEGER NOT NULL,
    workflow_name        TEXT NOT NULL,
    scope_type           TEXT NOT NULL,
    workflow_instance_id TEXT,
    ticket_id            TEXT,
    instructions_used    TEXT NOT NULL DEFAULT '',
    status               TEXT NOT NULL CHECK(status IN ('pending', 'running', 'completed', 'failed', 'skipped', 'canceled')),
    started_at           TEXT,
    ended_at             TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workflow_chain_run_steps_run ON workflow_chain_run_steps (chain_run_id, position);
