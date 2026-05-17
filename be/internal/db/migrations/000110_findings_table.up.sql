-- Create findings table replacing agent_sessions.findings JSON, workflow_instances.findings JSON,
-- and project_findings table with a unified (scope, scope_id, key) keyed store.
CREATE TABLE findings (
    id                   TEXT NOT NULL PRIMARY KEY,
    scope                TEXT NOT NULL CHECK(scope IN ('session','workflow_instance','project')),
    scope_id             TEXT NOT NULL,
    key                  TEXT NOT NULL,
    value                TEXT NOT NULL DEFAULT '',
    project_id           TEXT,
    workflow_instance_id TEXT,
    agent_type           TEXT,
    model_id             TEXT,
    created_at           TEXT NOT NULL,
    created_by           TEXT,
    created_source       TEXT,
    updated_at           TEXT NOT NULL,
    updated_by           TEXT,
    updated_source       TEXT,
    write_count          INTEGER NOT NULL DEFAULT 1,
    UNIQUE(scope, scope_id, key)
);

CREATE INDEX idx_findings_wfi_agent ON findings(workflow_instance_id, agent_type)
    WHERE workflow_instance_id IS NOT NULL;
CREATE INDEX idx_findings_project ON findings(project_id) WHERE scope='project';
CREATE INDEX idx_findings_scope ON findings(scope, scope_id);

-- Audit trail for every mutation.
CREATE TABLE findings_history (
    id           TEXT NOT NULL PRIMARY KEY,
    finding_id   TEXT REFERENCES findings(id) ON DELETE SET NULL,
    scope        TEXT NOT NULL,
    scope_id     TEXT NOT NULL,
    key          TEXT NOT NULL,
    operation    TEXT NOT NULL CHECK(operation IN ('add','append','delete')),
    old_value    TEXT,
    new_value    TEXT,
    actor_id     TEXT,
    actor_source TEXT,
    created_at   TEXT NOT NULL
);

CREATE INDEX idx_findings_history_scope ON findings_history(scope, scope_id, created_at DESC);
CREATE INDEX idx_findings_history_key   ON findings_history(scope, scope_id, key, created_at DESC);

-- Backfill from agent_sessions.findings (scope=session).
INSERT OR IGNORE INTO findings (
    id, scope, scope_id, key, value,
    project_id, workflow_instance_id, agent_type, model_id,
    created_at, created_source, updated_at, updated_source, write_count
)
SELECT
    lower(hex(randomblob(16))),
    'session',
    s.id,
    je.key,
    CASE je.type
        WHEN 'text'    THEN json_quote(je.value)
        WHEN 'null'    THEN 'null'
        WHEN 'true'    THEN 'true'
        WHEN 'false'   THEN 'false'
        WHEN 'integer' THEN CAST(je.value AS TEXT)
        WHEN 'real'    THEN CAST(je.value AS TEXT)
        ELSE je.value
    END,
    s.project_id,
    s.workflow_instance_id,
    s.agent_type,
    CASE WHEN s.model_id IS NOT NULL AND s.model_id != '' THEN s.model_id ELSE NULL END,
    COALESCE(s.created_at, datetime('now')),
    'system',
    COALESCE(s.updated_at, datetime('now')),
    'system',
    1
FROM agent_sessions s, json_each(s.findings) je
WHERE s.findings IS NOT NULL AND s.findings != '' AND s.findings != '{}';

-- Backfill from workflow_instances.findings (scope=workflow_instance).
INSERT OR IGNORE INTO findings (
    id, scope, scope_id, key, value,
    project_id, workflow_instance_id,
    created_at, created_source, updated_at, updated_source, write_count
)
SELECT
    lower(hex(randomblob(16))),
    'workflow_instance',
    wi.id,
    je.key,
    CASE je.type
        WHEN 'text'    THEN json_quote(je.value)
        WHEN 'null'    THEN 'null'
        WHEN 'true'    THEN 'true'
        WHEN 'false'   THEN 'false'
        WHEN 'integer' THEN CAST(je.value AS TEXT)
        WHEN 'real'    THEN CAST(je.value AS TEXT)
        ELSE je.value
    END,
    wi.project_id,
    wi.id,
    COALESCE(wi.created_at, datetime('now')),
    'system',
    COALESCE(wi.updated_at, datetime('now')),
    'system',
    1
FROM workflow_instances wi, json_each(wi.findings) je
WHERE wi.findings IS NOT NULL AND wi.findings != '' AND wi.findings != '{}';

-- Backfill from project_findings (scope=project).
INSERT OR IGNORE INTO findings (
    id, scope, scope_id, key, value,
    project_id,
    created_at, created_source, updated_at, updated_source, write_count
)
SELECT
    lower(hex(randomblob(16))),
    'project',
    pf.project_id,
    pf.key,
    pf.value,
    pf.project_id,
    COALESCE(pf.updated_at, datetime('now')),
    'system',
    COALESCE(pf.updated_at, datetime('now')),
    'system',
    1
FROM project_findings pf;

-- Drop old storage.
ALTER TABLE agent_sessions DROP COLUMN findings;
ALTER TABLE workflow_instances DROP COLUMN findings;
DROP TABLE project_findings;
