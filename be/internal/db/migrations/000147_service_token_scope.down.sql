-- Revert to project-only service tokens. Global-scope tokens (project_id NULL)
-- cannot exist in the restored NOT NULL schema, so they are dropped.
PRAGMA foreign_keys = OFF;

CREATE TABLE service_tokens_old (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    token_hash    TEXT NOT NULL UNIQUE,
    display_hint  TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    created_by    TEXT,
    last_used_at  TEXT
);

INSERT INTO service_tokens_old (id, project_id, name, token_hash, display_hint, created_at, created_by, last_used_at)
SELECT id, project_id, name, token_hash, display_hint, created_at, created_by, last_used_at
FROM service_tokens WHERE project_id IS NOT NULL;

DROP TABLE service_tokens;
ALTER TABLE service_tokens_old RENAME TO service_tokens;

CREATE INDEX IF NOT EXISTS idx_service_tokens_project ON service_tokens(project_id);
CREATE INDEX IF NOT EXISTS idx_service_tokens_hash ON service_tokens(token_hash);

PRAGMA foreign_keys = ON;
PRAGMA foreign_key_check;
