-- Drop the models.provider CHECK entirely: provider can now name any row in
-- custom_providers, whose set is dynamic (not known at migration time).
-- SQLite cannot ALTER a CHECK constraint, so rebuild the table (follows
-- 000186's recipe verbatim). Column set = 000186 base (19 cols) + 000191's
-- release_date (20 cols total). Keep CHECK(cli_model<>'' OR api_model<>'').
-- No FK to custom_providers: builtins (anthropic/openai/openrouter) aren't
-- rows in that table, so referential integrity for custom providers is
-- enforced at the service layer only (delete-in-use guard).
PRAGMA foreign_keys = OFF;

CREATE TABLE models_new (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
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
    release_date TEXT,
    CHECK (cli_model <> '' OR api_model <> '')
);

INSERT INTO models_new (
    id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
    cli_context, api_context, fallback_models, default_effort, read_only,
    enabled, created_at, updated_at, price_in, price_out, price_cache_write,
    price_cache_read, release_date
)
SELECT
    id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
    cli_context, api_context, fallback_models, default_effort, read_only,
    enabled, created_at, updated_at, price_in, price_out, price_cache_write,
    price_cache_read, release_date
FROM models;

DROP TABLE models;
ALTER TABLE models_new RENAME TO models;

PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;
