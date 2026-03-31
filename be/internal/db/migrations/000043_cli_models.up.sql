CREATE TABLE IF NOT EXISTS cli_models (
    id TEXT PRIMARY KEY,
    cli_type TEXT NOT NULL,
    display_name TEXT NOT NULL,
    mapped_model TEXT NOT NULL,
    reasoning_effort TEXT NOT NULL DEFAULT '',
    context_length INTEGER NOT NULL DEFAULT 200000,
    read_only INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (cli_type IN ('claude', 'opencode', 'codex'))
);

-- Seed system models (read_only = 1)
INSERT OR IGNORE INTO cli_models (id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, created_at, updated_at) VALUES
('opus', 'claude', 'Opus', 'opus', '', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('opus_1m', 'claude', 'Opus (1M)', 'opus[1m]', '', 1000000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('sonnet', 'claude', 'Sonnet', 'sonnet', '', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('haiku', 'claude', 'Haiku', 'haiku', '', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('opencode_gpt_normal', 'opencode', 'OpenCode GPT (Normal)', 'openai/gpt-5.3-codex', 'high', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('opencode_gpt_high', 'opencode', 'OpenCode GPT (High)', 'openai/gpt-5.3-codex', 'high', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('codex_gpt_normal', 'codex', 'Codex GPT (Normal)', 'gpt-5.3-codex', 'high', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('codex_gpt_high', 'codex', 'Codex GPT (High)', 'gpt-5.3-codex', 'high', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('codex_gpt54_normal', 'codex', 'Codex GPT-54 (Normal)', 'gpt-5.4', 'medium', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('codex_gpt54_high', 'codex', 'Codex GPT-54 (High)', 'gpt-5.4', 'high', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
