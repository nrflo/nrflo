CREATE TABLE IF NOT EXISTS ticket_refs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT    NOT NULL,
    ticket_id  TEXT    NOT NULL,
    kind       TEXT    NOT NULL,
    url        TEXT    NOT NULL,
    label      TEXT,
    created_at TEXT    NOT NULL,
    FOREIGN KEY (project_id, ticket_id) REFERENCES tickets(project_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_ticket_refs_ticket ON ticket_refs(project_id, ticket_id);
CREATE INDEX IF NOT EXISTS idx_ticket_refs_url    ON ticket_refs(url);
