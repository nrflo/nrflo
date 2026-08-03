PRAGMA foreign_keys = OFF;

CREATE TABLE tool_dispatches_old (
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

INSERT INTO tool_dispatches_old (id, project_id, session_id, tool_name, input, output, status, error_msg, duration_ms, created_at)
    SELECT id, project_id, session_id, tool_name, COALESCE(input, ''), output, status, error_msg, COALESCE(duration_ms, 0), created_at FROM tool_dispatches;

DROP TABLE tool_dispatches;
ALTER TABLE tool_dispatches_old RENAME TO tool_dispatches;

CREATE INDEX idx_tool_dispatches_lookup ON tool_dispatches (project_id, tool_name, created_at);

PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;
