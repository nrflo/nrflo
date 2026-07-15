-- Global workflow defs (workflows.is_global=1, stored under '__global__') could
-- never run project-scoped under a real project: the workflow_instances FK
-- (project_id, workflow_id) -> workflows(project_id, id) required the definition
-- to exist under the *executing* project. Split the two relationships:
--   def_project_id  — the project the definition resolved under ('__global__'
--                     for global defs); carries the workflows FK + CASCADE.
--   project_id      — the executing project; now FKs to projects directly so a
--                     project delete still removes instances that ran a global def.
-- def_project_id is nullable: a NULL skips composite-FK enforcement (SQLite
-- semantics), which keeps direct-SQL inserts (tests) valid; the repo always
-- stamps it (falling back to project_id).
PRAGMA foreign_keys = OFF;

CREATE TABLE workflow_instances_new (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    def_project_id  TEXT,
    ticket_id       TEXT NOT NULL DEFAULT '',
    workflow_id     TEXT NOT NULL,
    scope_type      TEXT NOT NULL DEFAULT 'ticket'
        CHECK (scope_type IN ('ticket', 'project')),
    status          TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'completed', 'failed', 'project_completed', 'waiting',
                           'planning', 'plan_ready', 'waiting_input', 'waiting_approval')),
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
    purge_on_completion INTEGER NOT NULL DEFAULT 0,
    launch_depth    INTEGER NOT NULL DEFAULT 0,
    parent_instance_id TEXT NOT NULL DEFAULT '',
    subworkflow_depth INTEGER NOT NULL DEFAULT 0,
    subworkflow_starts INTEGER NOT NULL DEFAULT 0,
    plan_auto_approve INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (def_project_id, workflow_id) REFERENCES workflows(project_id, id) ON DELETE CASCADE,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

-- Backfill: every existing row satisfied the old FK, so its definition lives
-- under its executing project.
INSERT INTO workflow_instances_new (
    id, project_id, def_project_id, ticket_id, workflow_id, scope_type, status,
    retry_count, parent_session, created_at, updated_at, skip_tags, worktree_path,
    branch_name, endless_loop, stop_endless_loop_after_iteration, scheduled_task_id,
    external_id, external_context, purge_on_completion, launch_depth,
    parent_instance_id, subworkflow_depth, subworkflow_starts, plan_auto_approve
)
SELECT
    id, project_id, project_id, ticket_id, workflow_id, scope_type, status,
    retry_count, parent_session, created_at, updated_at, skip_tags, worktree_path,
    branch_name, endless_loop, stop_endless_loop_after_iteration, scheduled_task_id,
    external_id, external_context, purge_on_completion, launch_depth,
    parent_instance_id, subworkflow_depth, subworkflow_starts, plan_auto_approve
FROM workflow_instances;

DROP TABLE workflow_instances;
ALTER TABLE workflow_instances_new RENAME TO workflow_instances;

CREATE INDEX idx_wfi_ticket ON workflow_instances(project_id, ticket_id);
CREATE INDEX idx_wfi_lookup ON workflow_instances(project_id, ticket_id, workflow_id, scope_type);
CREATE INDEX idx_workflow_instances_scheduled ON workflow_instances(scheduled_task_id);
CREATE INDEX idx_workflow_instances_external_id ON workflow_instances(external_id);
CREATE INDEX idx_wfi_def_workflow ON workflow_instances(def_project_id, workflow_id);

PRAGMA foreign_keys = ON;
PRAGMA foreign_key_check;
