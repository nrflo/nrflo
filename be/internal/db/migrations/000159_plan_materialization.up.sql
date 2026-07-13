-- Plan materialization: instance-scoped node/layer-policy tables written
-- exactly-once from an approved plan revision, plus the workflow_instances
-- status CHECK rebuild deferred by 000158 (planning/plan_ready/waiting_input/
-- waiting_approval — the plan-boundary suspend statuses).

CREATE TABLE IF NOT EXISTS workflow_instance_nodes (
    instance_id    TEXT    NOT NULL,
    node_id        TEXT    NOT NULL,
    layer          INTEGER NOT NULL,
    agent_type     TEXT    NOT NULL,
    instructions   TEXT    NOT NULL DEFAULT '',
    plan_revision  INTEGER NOT NULL,
    created_at     TEXT    NOT NULL,
    PRIMARY KEY (instance_id, node_id),
    FOREIGN KEY (instance_id) REFERENCES workflow_instances (id) ON DELETE CASCADE
);
CREATE INDEX idx_workflow_instance_nodes_layer ON workflow_instance_nodes(instance_id, layer);

CREATE TABLE IF NOT EXISTS workflow_instance_layer_policies (
    instance_id   TEXT    NOT NULL,
    layer         INTEGER NOT NULL,
    pass_policy   TEXT    NOT NULL DEFAULT 'any',
    PRIMARY KEY (instance_id, layer),
    FOREIGN KEY (instance_id) REFERENCES workflow_instances (id) ON DELETE CASCADE
);

-- Exactly-once materialization stamp on the plan head.
ALTER TABLE workflow_plans ADD COLUMN materialized_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE workflow_plans ADD COLUMN materialized_hash TEXT NOT NULL DEFAULT '';

-- Rebuild workflow_instances to widen the status CHECK (follows 000136's
-- recipe verbatim). Column set = 000136's 18 + purge_on_completion (000142)
-- + launch_depth (000154) + parent_instance_id/subworkflow_depth/subworkflow_starts (000155).
PRAGMA foreign_keys = OFF;

CREATE TABLE workflow_instances_new (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
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
    FOREIGN KEY (project_id, workflow_id) REFERENCES workflows(project_id, id) ON DELETE CASCADE
);

INSERT INTO workflow_instances_new (
    id, project_id, ticket_id, workflow_id, scope_type, status, retry_count,
    parent_session, created_at, updated_at, skip_tags, worktree_path, branch_name,
    endless_loop, stop_endless_loop_after_iteration, scheduled_task_id,
    external_id, external_context, purge_on_completion, launch_depth,
    parent_instance_id, subworkflow_depth, subworkflow_starts
)
SELECT
    id, project_id, ticket_id, workflow_id, scope_type, status, retry_count,
    parent_session, created_at, updated_at, skip_tags, worktree_path, branch_name,
    endless_loop, stop_endless_loop_after_iteration, scheduled_task_id,
    external_id, external_context, purge_on_completion, launch_depth,
    parent_instance_id, subworkflow_depth, subworkflow_starts
FROM workflow_instances;

DROP TABLE workflow_instances;
ALTER TABLE workflow_instances_new RENAME TO workflow_instances;

CREATE INDEX idx_wfi_ticket ON workflow_instances(project_id, ticket_id);
CREATE INDEX idx_wfi_lookup ON workflow_instances(project_id, ticket_id, workflow_id, scope_type);
CREATE INDEX idx_workflow_instances_scheduled ON workflow_instances(scheduled_task_id);
CREATE INDEX idx_workflow_instances_external_id ON workflow_instances(external_id);

PRAGMA foreign_keys = ON;
PRAGMA foreign_key_check;
