-- Drop override_system_prompt from cli_models. The per-model toggle is replaced by the
-- global claude_system_prompt_override_enabled setting (read freshly at spawn time).
-- Forward-only (no down file): rebuild the table without the column via the SQLite
-- table-rebuild pattern (cf. 000127). Keep CHECK(cli_type IN ('claude','codex')).

PRAGMA foreign_keys = OFF;

BEGIN;

CREATE TABLE cli_models_new (
    id               TEXT    PRIMARY KEY,
    cli_type         TEXT    NOT NULL,
    display_name     TEXT    NOT NULL,
    mapped_model     TEXT    NOT NULL,
    reasoning_effort TEXT    NOT NULL DEFAULT '',
    context_length   INTEGER NOT NULL DEFAULT 200000,
    read_only        INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT    NOT NULL,
    updated_at       TEXT    NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,
    CHECK (cli_type IN ('claude', 'codex'))
);

INSERT INTO cli_models_new
    SELECT id, cli_type, display_name, mapped_model, reasoning_effort,
           context_length, read_only, created_at, updated_at, enabled
    FROM cli_models;

DROP TABLE cli_models;
ALTER TABLE cli_models_new RENAME TO cli_models;

-- Seed the global toggle off (visibility only; GetConfig returns "" -> off on miss).
INSERT OR IGNORE INTO config (project_id, key, value)
    VALUES ('', 'claude_system_prompt_override_enabled', 'false');

COMMIT;

PRAGMA foreign_keys = ON;
