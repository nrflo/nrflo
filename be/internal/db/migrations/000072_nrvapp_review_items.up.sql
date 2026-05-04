CREATE TABLE IF NOT EXISTS nrvapp_review_items (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tool_name   TEXT NOT NULL,
    session_id  TEXT,
    input       TEXT NOT NULL,
    output      TEXT,
    draft       TEXT,
    status      TEXT NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'approved', 'rejected')),
    reject_reason TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    approved_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_nrvapp_review_items_lookup
    ON nrvapp_review_items (project_id, status, created_at DESC);
