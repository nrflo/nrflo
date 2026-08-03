-- Widen tool_dispatches into the single per-session tool-call audit: source
-- discriminates the invoke site (mcp/http/console/engine/python),
-- session_kind denormalizes agent_sessions.kind at write time, and
-- workflow_instance_id carries the calling session's bound run (empty when
-- none). input/duration_ms relax to nullable: the console-engine tap
-- (source='engine') has no tool_use_id to pair invoke/result, so it writes a
-- row with NULL input/output/duration_ms. SQLite has no ALTER COLUMN, so this
-- is a table rebuild (same shape as migration 000193).
PRAGMA foreign_keys = OFF;

CREATE TABLE tool_dispatches_new (
    id                   TEXT PRIMARY KEY,
    project_id           TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id           TEXT,
    tool_name            TEXT NOT NULL,
    input                TEXT,
    output               TEXT,
    status               TEXT NOT NULL CHECK (status IN ('success', 'error')),
    error_msg            TEXT,
    duration_ms          INTEGER,
    source               TEXT NOT NULL DEFAULT '',
    session_kind         TEXT NOT NULL DEFAULT '',
    workflow_instance_id TEXT NOT NULL DEFAULT '',
    created_at           TEXT NOT NULL
);

INSERT INTO tool_dispatches_new (id, project_id, session_id, tool_name, input, output, status, error_msg, duration_ms, created_at)
    SELECT id, project_id, session_id, tool_name, input, output, status, error_msg, duration_ms, created_at FROM tool_dispatches;

DROP TABLE tool_dispatches;
ALTER TABLE tool_dispatches_new RENAME TO tool_dispatches;

CREATE INDEX idx_tool_dispatches_lookup ON tool_dispatches (project_id, tool_name, created_at);
CREATE INDEX idx_tool_dispatches_session_tool ON tool_dispatches (session_id, tool_name);
CREATE INDEX idx_tool_dispatches_created_at ON tool_dispatches (created_at);

PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;
