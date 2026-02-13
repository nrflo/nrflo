-- SQLite <3.35 lacks ALTER TABLE DROP COLUMN, so recreate the table
CREATE TABLE chain_executions_backup AS SELECT
    id, project_id, name, status, workflow_name, category, created_by, created_at, updated_at
FROM chain_executions;

DROP TABLE chain_executions;

CREATE TABLE chain_executions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','completed','failed','canceled')),
    workflow_name TEXT NOT NULL,
    category TEXT,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT INTO chain_executions SELECT * FROM chain_executions_backup;
DROP TABLE chain_executions_backup;

DROP INDEX IF EXISTS idx_chain_exec_epic;
