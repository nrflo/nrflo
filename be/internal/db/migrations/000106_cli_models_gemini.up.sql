-- Add 'gemini' as a valid cli_type and seed three Gemini model rows.
-- SQLite requires table rebuild to modify CHECK constraints.

PRAGMA foreign_keys = OFF;

BEGIN;

CREATE TABLE cli_models_new (
    id               TEXT PRIMARY KEY,
    cli_type         TEXT NOT NULL,
    display_name     TEXT NOT NULL,
    mapped_model     TEXT NOT NULL,
    reasoning_effort TEXT NOT NULL DEFAULT '',
    context_length   INTEGER NOT NULL DEFAULT 200000,
    read_only        INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,
    CHECK (cli_type IN ('claude', 'opencode', 'codex', 'gemini'))
);

INSERT INTO cli_models_new
    SELECT id, cli_type, display_name, mapped_model, reasoning_effort,
           context_length, read_only, created_at, updated_at, enabled
    FROM cli_models;

DROP TABLE cli_models;
ALTER TABLE cli_models_new RENAME TO cli_models;

INSERT OR IGNORE INTO cli_models (id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at) VALUES
('gemini_pro',        'gemini', 'Gemini Pro',        'gemini-2.5-pro',        '', 1000000, 1, 1, '2026-05-15T00:00:00Z', '2026-05-15T00:00:00Z'),
('gemini_flash',      'gemini', 'Gemini Flash',      'gemini-2.5-flash',      '', 1000000, 1, 1, '2026-05-15T00:00:00Z', '2026-05-15T00:00:00Z'),
('gemini_flash_lite', 'gemini', 'Gemini Flash Lite', 'gemini-2.5-flash-lite', '', 1000000, 1, 1, '2026-05-15T00:00:00Z', '2026-05-15T00:00:00Z');

COMMIT;

PRAGMA foreign_keys = ON;
