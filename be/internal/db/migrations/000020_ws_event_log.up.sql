CREATE TABLE IF NOT EXISTS ws_event_log (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL,
    ticket_id TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    workflow TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ws_event_log_scope_seq ON ws_event_log(project_id, ticket_id, seq);
CREATE INDEX IF NOT EXISTS idx_ws_event_log_created_at ON ws_event_log(created_at);
