-- Registry of custom (BYO OpenAI-compatible) API providers: local/self-hosted
-- servers (Ollama, LM Studio, llama.cpp) or any other OpenAI-compatible
-- endpoint. api_wire selects the wire protocol: 'responses' (default, the
-- non-stateful /v1/responses API the openai provider already speaks — Ollama
-- >=0.13.3 and LM Studio >=0.3.29 support it) or 'chat_completions' (for
-- servers like llama.cpp's llama-server that only speak /v1/chat/completions).
-- No seed rows; models.provider references a row here dynamically (no FK —
-- see 000193).
CREATE TABLE custom_providers (
    name TEXT PRIMARY KEY,
    base_url TEXT NOT NULL,
    api_key TEXT NOT NULL DEFAULT '',
    api_wire TEXT NOT NULL DEFAULT 'responses' CHECK (api_wire IN ('responses', 'chat_completions')),
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
