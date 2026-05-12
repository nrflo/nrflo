-- Seed codex_gpt54_mini_low: gpt-5.4-mini with reasoning_effort=low.
-- Smaller, cheaper variant primarily used by the manual_testing harness
-- (test_codex.py default) to keep the suite cost-effective.
INSERT OR IGNORE INTO cli_models (id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, created_at, updated_at)
VALUES (
    'codex_gpt54_mini_low',
    'codex',
    'Codex GPT-54 Mini (Low)',
    'gpt-5.4-mini',
    'low',
    200000,
    1,
    '2026-05-12T00:00:00Z',
    '2026-05-12T00:00:00Z'
);
