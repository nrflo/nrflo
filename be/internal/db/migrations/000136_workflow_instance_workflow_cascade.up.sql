-- The workflow_instances -> workflows composite FK was ON DELETE RESTRICT,
-- which blocked deleting a project (its workflows cascade-delete cannot proceed
-- while instances reference them). Rebuild the table with ON DELETE CASCADE so a
-- project (and a workflow definition) delete cleanly removes its instances.
PRAGMA foreign_keys = OFF;

CREATE TABLE workflow_instances_new (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    ticket_id       TEXT NOT NULL DEFAULT '',
    workflow_id     TEXT NOT NULL,
    scope_type      TEXT NOT NULL DEFAULT 'ticket'
        CHECK (scope_type IN ('ticket', 'project')),
    status          TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'completed', 'failed', 'project_completed', 'waiting')),
    retry_count     INTEGER NOT NULL DEFAULT 0,
    parent_session  TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    skip_tags       TEXT NOT NULL DEFAULT '[]',
    worktree_path   TEXT,
    branch_name     TEXT,
    endless_loop    INTEGER NOT NULL DEFAULT 0,
    stop_endless_loop_after_iteration INTEGER NOT NULL DEFAULT 0,
    scheduled_task_id TEXT REFERENCES scheduled_tasks(id) ON DELETE SET NULL,
    external_id     TEXT,
    external_context TEXT,
    FOREIGN KEY (project_id, workflow_id) REFERENCES workflows(project_id, id) ON DELETE CASCADE
);

INSERT INTO workflow_instances_new (
    id, project_id, ticket_id, workflow_id, scope_type, status, retry_count,
    parent_session, created_at, updated_at, skip_tags, worktree_path, branch_name,
    endless_loop, stop_endless_loop_after_iteration, scheduled_task_id,
    external_id, external_context
)
SELECT
    id, project_id, ticket_id, workflow_id, scope_type, status, retry_count,
    parent_session, created_at, updated_at, skip_tags, worktree_path, branch_name,
    endless_loop, stop_endless_loop_after_iteration, scheduled_task_id,
    external_id, external_context
FROM workflow_instances;

DROP TABLE workflow_instances;
ALTER TABLE workflow_instances_new RENAME TO workflow_instances;

CREATE INDEX idx_wfi_ticket ON workflow_instances(project_id, ticket_id);
CREATE INDEX idx_wfi_lookup ON workflow_instances(project_id, ticket_id, workflow_id, scope_type);
CREATE INDEX idx_workflow_instances_scheduled ON workflow_instances(scheduled_task_id);
CREATE INDEX idx_workflow_instances_external_id ON workflow_instances(external_id);

PRAGMA foreign_keys = ON;
PRAGMA foreign_key_check;
