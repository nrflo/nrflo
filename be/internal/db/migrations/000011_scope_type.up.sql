-- Add scope_type to workflows and workflow_instances tables
-- scope_type: "ticket" (default) or "project"
-- Also removes FK to tickets on workflow_instances so project-scoped workflows
-- (which have no ticket_id) can be created without violating referential integrity.

PRAGMA foreign_keys = OFF;

-- 1. Add scope_type to workflows table
ALTER TABLE workflows ADD COLUMN scope_type TEXT NOT NULL DEFAULT 'ticket'
    CHECK (scope_type IN ('ticket', 'project'));

-- 2. Rebuild workflow_instances: add scope_type, remove FK to tickets
CREATE TABLE workflow_instances_new (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    ticket_id       TEXT NOT NULL DEFAULT '',
    workflow_id     TEXT NOT NULL,
    scope_type      TEXT NOT NULL DEFAULT 'ticket'
        CHECK (scope_type IN ('ticket', 'project')),
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
    FOREIGN KEY (project_id, workflow_id) REFERENCES workflows(project_id, id) ON DELETE RESTRICT
);

INSERT INTO workflow_instances_new (
    id, project_id, ticket_id, workflow_id, scope_type, status, category,
    current_phase, phase_order, phases, findings, retry_count, parent_session,
    created_at, updated_at
)
SELECT
    id, project_id, ticket_id, workflow_id, 'ticket', status, category,
    current_phase, phase_order, phases, findings, retry_count, parent_session,
    created_at, updated_at
FROM workflow_instances;

DROP TABLE workflow_instances;
ALTER TABLE workflow_instances_new RENAME TO workflow_instances;

-- Recreate indexes
CREATE UNIQUE INDEX idx_wfi_unique ON workflow_instances(project_id, ticket_id, workflow_id, scope_type);
CREATE INDEX idx_wfi_ticket ON workflow_instances(project_id, ticket_id);

-- Recreate agent_sessions FK (it references workflow_instances.id)
-- agent_sessions already has the FK from migration 000004, but since we rebuilt
-- the table, SQLite should still see the same id PRIMARY KEY.

PRAGMA foreign_keys = ON;
PRAGMA foreign_key_check;
