CREATE TABLE IF NOT EXISTS notification_channels (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('slack', 'telegram')),
    enabled    INTEGER NOT NULL DEFAULT 1,
    config     TEXT NOT NULL DEFAULT '{}',
    event_types TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notification_channels_project ON notification_channels (project_id);

CREATE TABLE IF NOT EXISTS notification_deliveries (
    id              TEXT PRIMARY KEY,
    channel_id      TEXT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    project_id      TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    payload         TEXT NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed', 'giving_up')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT '',
    next_attempt_at TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_status ON notification_deliveries (status, next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_notification_deliveries_channel ON notification_deliveries (channel_id, created_at DESC);
