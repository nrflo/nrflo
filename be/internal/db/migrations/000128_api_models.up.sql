CREATE TABLE api_models (
    id                TEXT    PRIMARY KEY,
    provider          TEXT    NOT NULL,
    display_name      TEXT    NOT NULL,
    mapped_model      TEXT    NOT NULL,
    reasoning_effort  TEXT    NOT NULL DEFAULT '',
    context_length    INTEGER NOT NULL DEFAULT 200000,
    read_only         INTEGER NOT NULL DEFAULT 0,
    enabled           INTEGER NOT NULL DEFAULT 1,
    created_at        TEXT    NOT NULL,
    updated_at        TEXT    NOT NULL,
    CHECK (provider IN ('anthropic', 'openai'))
);

-- Seed read-only Anthropic rows
INSERT INTO api_models (id, provider, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at) VALUES
    ('opus_4_7',     'anthropic', 'Claude Opus 4.7',       'claude-opus-4-7',     '', 200000,  1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
    ('opus_4_7_1m',  'anthropic', 'Claude Opus 4.7 (1M)',  'claude-opus-4-7[1m]', '', 1000000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
    ('opus_4_6',     'anthropic', 'Claude Opus 4.6',       'claude-opus-4-6',     '', 200000,  1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
    ('opus_4_6_1m',  'anthropic', 'Claude Opus 4.6 (1M)',  'claude-opus-4-6[1m]', '', 1000000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
    ('sonnet',       'anthropic', 'Claude Sonnet 4.6',     'claude-sonnet-4-6',   '', 200000,  1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
    ('haiku',        'anthropic', 'Claude Haiku 4.5',      'claude-haiku-4-5',    '', 200000,  1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');

-- Seed read-only OpenAI rows
INSERT INTO api_models (id, provider, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at) VALUES
    ('gpt53_codex_high',   'openai', 'GPT-5.3 Codex (High)',   'gpt-5.3-codex', 'high',   200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
    ('gpt53_codex_medium', 'openai', 'GPT-5.3 Codex (Medium)', 'gpt-5.3-codex', 'medium', 200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
    ('gpt53_codex_low',    'openai', 'GPT-5.3 Codex (Low)',    'gpt-5.3-codex', 'low',    200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
    ('gpt54_high',         'openai', 'GPT-5.4 (High)',         'gpt-5.4',       'high',   200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
    ('gpt54_medium',       'openai', 'GPT-5.4 (Medium)',       'gpt-5.4',       'medium', 200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
    ('gpt54_low',          'openai', 'GPT-5.4 (Low)',          'gpt-5.4',       'low',    200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
