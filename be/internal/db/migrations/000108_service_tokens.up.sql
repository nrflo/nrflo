CREATE TABLE IF NOT EXISTS service_tokens (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    token_hash    TEXT NOT NULL UNIQUE,
    display_hint  TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    created_by    TEXT,
    last_used_at  TEXT
);

CREATE INDEX IF NOT EXISTS idx_service_tokens_project ON service_tokens(project_id);
CREATE INDEX IF NOT EXISTS idx_service_tokens_hash ON service_tokens(token_hash);
