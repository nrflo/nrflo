-- Widen models.provider CHECK to include 'openrouter'. SQLite cannot ALTER a
-- CHECK constraint, so rebuild the table (follows 000159's recipe verbatim).
-- Column set = 000167 base (17 cols) + 000183 pricing (4 cols) = 19 cols.
-- No indexes on models and no FK references models, so nothing else to
-- recreate. No seeded openrouter rows.
PRAGMA foreign_keys = OFF;

CREATE TABLE models_new (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider IN ('anthropic', 'openai', 'openrouter')),
    display_name TEXT NOT NULL,
    cli_model TEXT NOT NULL DEFAULT '',
    api_model TEXT NOT NULL DEFAULT '',
    cli_efforts TEXT NOT NULL DEFAULT '[]',
    api_efforts TEXT NOT NULL DEFAULT '[]',
    cli_context INTEGER NOT NULL DEFAULT 200000,
    api_context INTEGER NOT NULL DEFAULT 200000,
    fallback_models TEXT NOT NULL DEFAULT '',
    default_effort TEXT NOT NULL DEFAULT '',
    read_only INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    price_in REAL,
    price_out REAL,
    price_cache_write REAL,
    price_cache_read REAL,
    CHECK (cli_model <> '' OR api_model <> '')
);

INSERT INTO models_new (
    id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
    cli_context, api_context, fallback_models, default_effort, read_only,
    enabled, created_at, updated_at, price_in, price_out, price_cache_write, price_cache_read
)
SELECT
    id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
    cli_context, api_context, fallback_models, default_effort, read_only,
    enabled, created_at, updated_at, price_in, price_out, price_cache_write, price_cache_read
FROM models;

DROP TABLE models;
ALTER TABLE models_new RENAME TO models;

PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;
