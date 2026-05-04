CREATE TABLE IF NOT EXISTS nrvapp_config_versions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    file       TEXT NOT NULL,
    version    INTEGER NOT NULL,
    content    BLOB NOT NULL,
    actor      TEXT,
    created_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_nrvapp_config_versions_unique
    ON nrvapp_config_versions (project_id, file, version);
