-- Re-add categories/category columns to all three tables.

-- 1. Simple add: workflows.categories
ALTER TABLE workflows ADD COLUMN categories TEXT;

-- 2. Recreate workflow_instances with category column re-added
PRAGMA foreign_keys = OFF;

CREATE TABLE workflow_instances_new (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL,
    ticket_id       TEXT NOT NULL DEFAULT '',
    workflow_id     TEXT NOT NULL,
    scope_type      TEXT NOT NULL DEFAULT 'ticket'
        CHECK (scope_type IN ('ticket', 'project')),
    status          TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'completed', 'failed', 'project_completed')),
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
    id, project_id, ticket_id, workflow_id, scope_type, status,
    current_phase, phase_order, phases, findings, retry_count, parent_session,
    created_at, updated_at
)
SELECT
    id, project_id, ticket_id, workflow_id, scope_type, status,
    current_phase, phase_order, phases, findings, retry_count, parent_session,
    created_at, updated_at
FROM workflow_instances;

DROP TABLE workflow_instances;
ALTER TABLE workflow_instances_new RENAME TO workflow_instances;

CREATE UNIQUE INDEX idx_wfi_unique ON workflow_instances(project_id, ticket_id, workflow_id, scope_type);
CREATE INDEX idx_wfi_ticket ON workflow_instances(project_id, ticket_id);

-- Must also rebuild agent_sessions since it has FK to workflow_instances
CREATE TABLE agent_sessions_new (
    id                    TEXT PRIMARY KEY,
    project_id            TEXT NOT NULL,
    ticket_id             TEXT NOT NULL,
    workflow_instance_id  TEXT NOT NULL,
    phase                 TEXT NOT NULL,
    agent_type            TEXT NOT NULL,
    model_id              TEXT,
    status                TEXT NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'completed', 'failed', 'timeout', 'continued', 'project_completed', 'callback')),
    result                TEXT
        CHECK (result IS NULL OR result IN ('pass', 'fail', 'continue', 'timeout', 'callback')),
    result_reason         TEXT,
    pid                   INTEGER,
    findings              TEXT,
    context_left          INTEGER,
    ancestor_session_id   TEXT,
    spawn_command         TEXT,
    prompt_context        TEXT,
    restart_count         INTEGER NOT NULL DEFAULT 0,
    started_at            TEXT,
    ended_at              TEXT,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL,
    FOREIGN KEY (workflow_instance_id) REFERENCES workflow_instances(id) ON DELETE CASCADE,
    FOREIGN KEY (ancestor_session_id) REFERENCES agent_sessions(id) ON DELETE RESTRICT
);

INSERT INTO agent_sessions_new (
    id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id,
    status, result, result_reason, pid, findings, context_left, ancestor_session_id,
    spawn_command, prompt_context, restart_count, started_at, ended_at,
    created_at, updated_at
)
SELECT
    id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id,
    status, result, result_reason, pid, findings, context_left, ancestor_session_id,
    spawn_command, prompt_context, restart_count, started_at, ended_at,
    created_at, updated_at
FROM agent_sessions;

DROP TABLE agent_sessions;
ALTER TABLE agent_sessions_new RENAME TO agent_sessions;

CREATE INDEX idx_agent_sessions_project_ticket ON agent_sessions(project_id, ticket_id);
CREATE INDEX idx_agent_sessions_ticket_phase ON agent_sessions(ticket_id, phase);
CREATE INDEX idx_agent_sessions_wfi ON agent_sessions(workflow_instance_id);
CREATE INDEX idx_agent_sessions_wfi_status ON agent_sessions(workflow_instance_id, status);

-- 3. Recreate chain_executions with category column re-added
CREATE TABLE chain_executions_backup AS SELECT
    id, project_id, name, status, workflow_name, epic_ticket_id, created_by, created_at, updated_at
FROM chain_executions;

DROP TABLE chain_executions;

CREATE TABLE chain_executions (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    name          TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'canceled')),
    workflow_name TEXT NOT NULL,
    category      TEXT NOT NULL DEFAULT '',
    epic_ticket_id TEXT,
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

INSERT INTO chain_executions (
    id, project_id, name, status, workflow_name, epic_ticket_id, created_by, created_at, updated_at
)
SELECT * FROM chain_executions_backup;
DROP TABLE chain_executions_backup;

CREATE INDEX idx_chain_exec_project_status ON chain_executions(project_id, status);
CREATE INDEX idx_chain_exec_epic ON chain_executions(project_id, epic_ticket_id);

PRAGMA foreign_keys = ON;
PRAGMA foreign_key_check;
