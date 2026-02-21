CREATE TABLE preferences (
    name       TEXT PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
