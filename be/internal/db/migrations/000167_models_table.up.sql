CREATE TABLE models (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider IN ('anthropic', 'openai')),
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
    CHECK (cli_model <> '' OR api_model <> '')
);

INSERT INTO models (
    id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
    cli_context, api_context, fallback_models, default_effort, read_only,
    enabled, created_at, updated_at
) VALUES
('sonnet-5', 'anthropic', 'Claude Sonnet 5', 'claude-sonnet-5', 'claude-sonnet-5', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 1000000, 1000000, '', '', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('haiku-4-5', 'anthropic', 'Claude Haiku 4.5', 'claude-haiku-4-5', 'claude-haiku-4-5', '["low","medium","high"]', '["low","medium","high"]', 200000, 200000, '', '', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('opus-4-6', 'anthropic', 'Claude Opus 4.6', 'claude-opus-4-6', 'claude-opus-4-6', '["low","medium","high","max"]', '["low","medium","high","max"]', 200000, 1000000, '', '', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('opus-4-6-1m', 'anthropic', 'Claude Opus 4.6 (1M)', 'claude-opus-4-6[1m]', 'claude-opus-4-6[1m]', '["low","medium","high","max"]', '["low","medium","high","max"]', 1000000, 1000000, 'claude-opus-4-6', '', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('opus-4-7', 'anthropic', 'Claude Opus 4.7', 'claude-opus-4-7', 'claude-opus-4-7', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 200000, 1000000, '', '', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('opus-4-7-1m', 'anthropic', 'Claude Opus 4.7 (1M)', 'claude-opus-4-7[1m]', 'claude-opus-4-7[1m]', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 1000000, 1000000, 'claude-opus-4-7', '', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('opus-4-8', 'anthropic', 'Claude Opus 4.8', 'claude-opus-4-8', 'claude-opus-4-8', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 200000, 1000000, '', '', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('opus-4-8-1m', 'anthropic', 'Claude Opus 4.8 (1M)', 'claude-opus-4-8[1m]', 'claude-opus-4-8[1m]', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 1000000, 1000000, 'claude-opus-4-8', '', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('gpt-5.3-codex', 'openai', 'GPT-5.3 Codex', 'gpt-5.3-codex', 'gpt-5.3-codex', '["low","medium","high","xhigh"]', '["low","medium","high","xhigh"]', 200000, 200000, '', 'high', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('gpt-5.4', 'openai', 'GPT-5.4', 'gpt-5.4', 'gpt-5.4', '["low","medium","high","xhigh"]', '["low","medium","high","xhigh"]', 200000, 200000, '', 'medium', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('gpt-5.4-mini', 'openai', 'GPT-5.4 Mini', 'gpt-5.4-mini', '', '["low","medium","high","xhigh"]', '[]', 200000, 200000, '', 'low', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('gpt-5.5', 'openai', 'GPT-5.5', 'gpt-5.5', 'gpt-5.5', '["low","medium","high","xhigh"]', '["low","medium","high","xhigh"]', 200000, 200000, '', 'medium', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('gpt-5.5-mini', 'openai', 'GPT-5.5 Mini', 'gpt-5.5-mini', '', '["low","medium","high","xhigh"]', '[]', 200000, 200000, '', 'low', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('gpt-5.6-sol', 'openai', 'GPT-5.6 Sol', 'gpt-5.6-sol', 'gpt-5.6-sol', '["low","medium","high","xhigh","max","ultra"]', '["low","medium","high","xhigh","max"]', 372000, 372000, '', 'medium', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('gpt-5.6-terra', 'openai', 'GPT-5.6 Terra', 'gpt-5.6-terra', '', '["low","medium","high","xhigh","max","ultra"]', '[]', 372000, 200000, '', 'medium', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('gpt-5.6-luna', 'openai', 'GPT-5.6 Luna', 'gpt-5.6-luna', '', '["low","medium","high","xhigh","max"]', '[]', 372000, 200000, '', 'low', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z');

INSERT INTO models (
    id, provider, display_name, cli_model, cli_efforts, cli_context,
    fallback_models, default_effort, read_only, enabled, created_at, updated_at
)
SELECT id, CASE cli_type WHEN 'claude' THEN 'anthropic' ELSE 'openai' END,
       display_name, mapped_model, supported_efforts, context_length,
       fallback_models, reasoning_effort, 0, enabled, created_at, updated_at
FROM cli_models WHERE read_only = 0
ON CONFLICT(id) DO NOTHING;

INSERT INTO models (
    id, provider, display_name, api_model, api_efforts, api_context,
    default_effort, read_only, enabled, created_at, updated_at
)
SELECT id, provider, display_name, mapped_model, supported_efforts,
       context_length, reasoning_effort, 0, enabled, created_at, updated_at
FROM api_models WHERE read_only = 0 AND true
ON CONFLICT(id) DO UPDATE SET
    api_model = excluded.api_model,
    api_efforts = excluded.api_efforts,
    api_context = excluded.api_context
WHERE models.read_only = 0;
