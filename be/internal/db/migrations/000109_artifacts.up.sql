CREATE TABLE IF NOT EXISTS artifacts (
    id                   TEXT PRIMARY KEY,
    project_id           TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    workflow_instance_id TEXT NOT NULL REFERENCES workflow_instances(id) ON DELETE CASCADE,
    name                 TEXT NOT NULL,
    type                 TEXT NOT NULL CHECK (type IN ('internal', 's3', 'cloudflare_r2')),
    path_key             TEXT NOT NULL,
    size_bytes           INTEGER NOT NULL DEFAULT 0,
    content_type         TEXT,
    source               TEXT NOT NULL CHECK (source IN ('input', 'agent')),
    created_by_session   TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL,
    UNIQUE(workflow_instance_id, name)
);

CREATE INDEX IF NOT EXISTS idx_artifacts_wfi ON artifacts(workflow_instance_id);
CREATE INDEX IF NOT EXISTS idx_artifacts_project ON artifacts(project_id);
