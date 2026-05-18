CREATE TABLE IF NOT EXISTS review_items (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tool_name     TEXT NOT NULL,
    session_id    TEXT,
    input         TEXT NOT NULL,
    output        TEXT,
    draft         TEXT,
    status        TEXT NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'approved', 'rejected')),
    reject_reason TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    approved_at   TEXT
);

CREATE INDEX IF NOT EXISTS idx_review_items_lookup
    ON review_items (project_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS customer_config_versions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    file       TEXT NOT NULL,
    version    INTEGER NOT NULL,
    content    BLOB NOT NULL,
    actor      TEXT,
    created_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_config_versions_unique
    ON customer_config_versions (project_id, file, version);

DROP INDEX IF EXISTS python_scripts_tool_name;

CREATE TABLE python_scripts_backup (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    code        TEXT NOT NULL DEFAULT '',
    file_path   TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
INSERT INTO python_scripts_backup
    SELECT id, project_id, name, description, code, file_path, created_at, updated_at
    FROM python_scripts;
DROP TABLE python_scripts;
ALTER TABLE python_scripts_backup RENAME TO python_scripts;
CREATE UNIQUE INDEX python_scripts_project_id_id ON python_scripts(project_id, id);
