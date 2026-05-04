CREATE TABLE IF NOT EXISTS nrvapp_tool_dispatches (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id  TEXT,
    tool_name   TEXT NOT NULL,
    input       TEXT NOT NULL,
    output      TEXT,
    status      TEXT NOT NULL CHECK (status IN ('success', 'error')),
    error_msg   TEXT,
    duration_ms INTEGER NOT NULL,
    created_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_nrvapp_tool_dispatches_lookup
    ON nrvapp_tool_dispatches (project_id, tool_name, created_at);
