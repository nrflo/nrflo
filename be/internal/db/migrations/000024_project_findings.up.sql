CREATE TABLE project_findings (
    project_id  TEXT NOT NULL,
    key         TEXT NOT NULL,
    value       TEXT NOT NULL DEFAULT '',
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (project_id, key),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);
