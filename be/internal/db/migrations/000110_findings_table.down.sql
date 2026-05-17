-- Best-effort rollback. Re-adds JSON blob columns, recreates project_findings,
-- and reflushes data. History is intentionally lost.

ALTER TABLE agent_sessions ADD COLUMN findings TEXT;
ALTER TABLE workflow_instances ADD COLUMN findings TEXT;

CREATE TABLE project_findings (
    project_id TEXT NOT NULL,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    PRIMARY KEY (project_id, key),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

-- Reflow session findings into JSON blob.
UPDATE agent_sessions SET findings = (
    SELECT json_group_object(f.key, json(f.value))
    FROM findings f
    WHERE f.scope = 'session' AND f.scope_id = agent_sessions.id
);

-- Reflow workflow-instance findings into JSON blob.
UPDATE workflow_instances SET findings = (
    SELECT json_group_object(f.key, json(f.value))
    FROM findings f
    WHERE f.scope = 'workflow_instance' AND f.scope_id = workflow_instances.id
);

-- Reflow project findings.
INSERT INTO project_findings (project_id, key, value, updated_at)
SELECT f.project_id, f.key, f.value, f.updated_at
FROM findings f
WHERE f.scope = 'project';

DROP TABLE findings_history;
DROP TABLE findings;
