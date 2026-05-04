CREATE TABLE IF NOT EXISTS users (
    id                  TEXT PRIMARY KEY,
    email               TEXT NOT NULL UNIQUE COLLATE NOCASE,
    display_name        TEXT NOT NULL DEFAULT '',
    password_hash       TEXT NOT NULL,
    role                TEXT NOT NULL DEFAULT 'viewer' CHECK (role IN ('admin', 'viewer')),
    status              TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    must_change_password INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    last_login_at       TEXT
);

CREATE INDEX IF NOT EXISTS idx_users_status ON users (status);
