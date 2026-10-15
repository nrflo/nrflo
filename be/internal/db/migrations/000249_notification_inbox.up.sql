CREATE TABLE notification_inbox (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL DEFAULT '',
    ticket_id TEXT NOT NULL DEFAULT '',
    workflow_id TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TEXT NOT NULL,
    read_at TEXT
);
CREATE INDEX idx_notification_inbox_scope ON notification_inbox(project_id, created_at DESC);
CREATE INDEX idx_notification_inbox_unread ON notification_inbox(read_at);
