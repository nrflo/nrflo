-- Remove scope_type from workflows and workflow_instances
-- Rebuild workflow_instances to restore FK to tickets

PRAGMA foreign_keys = OFF;

-- Delete project-scoped instances (they have no ticket_id)
DELETE FROM agent_sessions WHERE workflow_instance_id IN (
    SELECT id FROM workflow_instances WHERE scope_type = 'project'
);
DELETE FROM workflow_instances WHERE scope_type = 'project';
DELETE FROM workflows WHERE scope_type = 'project';

-- Rebuild workflow_instances without scope_type, restoring FK to tickets
CREATE TABLE workflow_instances_new (
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

INSERT INTO workflow_instances_new (
    id, project_id, ticket_id, workflow_id, status, category,
    current_phase, phase_order, phases, findings, retry_count, parent_session,
    created_at, updated_at
)
SELECT
    id, project_id, ticket_id, workflow_id, status, category,
    current_phase, phase_order, phases, findings, retry_count, parent_session,
    created_at, updated_at
FROM workflow_instances;

DROP TABLE workflow_instances;
ALTER TABLE workflow_instances_new RENAME TO workflow_instances;

CREATE UNIQUE INDEX idx_wfi_unique ON workflow_instances(project_id, ticket_id, workflow_id);
CREATE INDEX idx_wfi_ticket ON workflow_instances(project_id, ticket_id);

-- Remove scope_type from workflows (rebuild needed for older SQLite)
CREATE TABLE workflows_new (
    id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    description TEXT,
    categories TEXT,
    phases TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (project_id, id),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

INSERT INTO workflows_new SELECT id, project_id, description, categories, phases, created_at, updated_at FROM workflows;
DROP TABLE workflows;
ALTER TABLE workflows_new RENAME TO workflows;
CREATE INDEX idx_workflows_project ON workflows(project_id);

PRAGMA foreign_keys = ON;
