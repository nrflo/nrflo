-- SQLite cannot ALTER CHECK constraints, so recreate the table with an expanded kind check.
CREATE TABLE notification_channels_new (
    id               TEXT NOT NULL PRIMARY KEY,
    project_id       TEXT NOT NULL,
    workflow_id      TEXT NOT NULL,
    name             TEXT NOT NULL,
    kind             TEXT NOT NULL CHECK (kind IN ('slack', 'telegram', 'script')),
    enabled          INTEGER NOT NULL DEFAULT 1,
    config           TEXT NOT NULL DEFAULT '{}',
    message_template TEXT NOT NULL DEFAULT '',
    event_types      TEXT NOT NULL DEFAULT '[]',
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    FOREIGN KEY (project_id, workflow_id) REFERENCES workflows(project_id, id) ON DELETE CASCADE
);

INSERT INTO notification_channels_new
    (id, project_id, workflow_id, name, kind, enabled, config, message_template, event_types, created_at, updated_at)
SELECT id, project_id, workflow_id, name, kind, enabled, config, message_template, event_types, created_at, updated_at
FROM notification_channels;

DROP TABLE notification_channels;

ALTER TABLE notification_channels_new RENAME TO notification_channels;

CREATE INDEX IF NOT EXISTS idx_notification_channels_workflow ON notification_channels (project_id, workflow_id);
