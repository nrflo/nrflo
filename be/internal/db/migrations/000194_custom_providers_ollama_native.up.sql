-- Widen custom_providers.api_wire to add 'ollama_native': Ollama's native
-- NDJSON POST /api/chat wire, which lets nrflo send think:false to disable
-- hybrid-thinking models (unlike the OpenAI-compatible /v1 wires). SQLite
-- can't ALTER a CHECK constraint, so rebuild the table (follows 000193's
-- recipe verbatim). No FK children (see 000192/000193).
PRAGMA foreign_keys = OFF;

CREATE TABLE custom_providers_new (
    name TEXT PRIMARY KEY,
    base_url TEXT NOT NULL,
    api_key TEXT NOT NULL DEFAULT '',
    api_wire TEXT NOT NULL DEFAULT 'responses' CHECK (api_wire IN ('responses', 'chat_completions', 'ollama_native')),
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT INTO custom_providers_new (name, base_url, api_key, api_wire, enabled, created_at, updated_at)
SELECT name, base_url, api_key, api_wire, enabled, created_at, updated_at
FROM custom_providers;

DROP TABLE custom_providers;
ALTER TABLE custom_providers_new RENAME TO custom_providers;

PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;
