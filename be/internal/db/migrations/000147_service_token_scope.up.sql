-- Service tokens gain an explicit scope: 'project' (bound to one project, the
-- existing behavior) or 'global' (all projects; project_id NULL). project_id
-- becomes nullable so a global token has no owning project. Requires a table
-- rebuild (SQLite cannot drop the NOT NULL / add the CHECK in place).
PRAGMA foreign_keys = OFF;

CREATE TABLE service_tokens_new (
    id            TEXT PRIMARY KEY,
    project_id    TEXT REFERENCES projects(id) ON DELETE CASCADE,
    scope         TEXT NOT NULL DEFAULT 'project' CHECK (scope IN ('project', 'global')),
    name          TEXT NOT NULL,
    token_hash    TEXT NOT NULL UNIQUE,
    display_hint  TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    created_by    TEXT,
    last_used_at  TEXT
);

INSERT INTO service_tokens_new (id, project_id, scope, name, token_hash, display_hint, created_at, created_by, last_used_at)
SELECT id, project_id, 'project', name, token_hash, display_hint, created_at, created_by, last_used_at
FROM service_tokens;

DROP TABLE service_tokens;
ALTER TABLE service_tokens_new RENAME TO service_tokens;

CREATE INDEX IF NOT EXISTS idx_service_tokens_project ON service_tokens(project_id);
CREATE INDEX IF NOT EXISTS idx_service_tokens_hash ON service_tokens(token_hash);

PRAGMA foreign_keys = ON;
PRAGMA foreign_key_check;
