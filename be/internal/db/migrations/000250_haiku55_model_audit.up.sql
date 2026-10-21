-- Model audit 2026-10-21. Haiku 5.5 settings and prices from
-- platform.claude.com/docs/en/about-claude/models/overview and .../pricing
-- (≤100k-token prompt rates); it replaces the stale Haiku 4.5.
INSERT INTO models (
    id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
    cli_context, api_context, fallback_models, default_effort, read_only,
    enabled, created_at, updated_at, release_date,
    price_in, price_out, price_cache_write, price_cache_read
) VALUES (
    'haiku-5-5', 'anthropic', 'Claude Haiku 5.5', 'claude-haiku-5-5', 'claude-haiku-5-5',
    '["low","medium","high","xhigh","max"]',
    '["low","medium","high","xhigh","max"]',
    1000000, 1000000, '', '', 1, 1,
    '2026-10-21T00:00:00Z', '2026-10-21T00:00:00Z', '2026-10-07',
    0.1, 0.5, 0.125, 0.01
);

UPDATE agent_definitions SET model = 'haiku-5-5' WHERE model = 'haiku-4-5';
UPDATE agent_definitions SET low_consumption_model = 'haiku-5-5' WHERE low_consumption_model = 'haiku-4-5';
UPDATE system_agent_definitions SET model = 'haiku-5-5' WHERE model = 'haiku-4-5';
UPDATE workflows SET observer_model = 'haiku-5-5' WHERE observer_model = 'haiku-4-5';
UPDATE config SET value = 'haiku-5-5' WHERE key = 'observer_model' AND value = 'haiku-4-5';
UPDATE tier_models SET model_id = 'haiku-5-5' WHERE model_id = 'haiku-4-5';
DELETE FROM models WHERE id = 'haiku-4-5';

UPDATE models SET price_cache_read = 0.1 WHERE id = 'sonnet-5-5';

-- Codex model/list no longer ships these; gpt-5.5-mini was CLI-only.
UPDATE models SET cli_model = '', cli_efforts = '[]' WHERE id IN ('gpt-5.4', 'gpt-5.4-mini', 'gpt-5.5');
DELETE FROM models WHERE id = 'gpt-5.5-mini';

-- developers.openai.com/api/docs/pricing; cache write follows the 1.25x seed convention.
UPDATE models SET price_in = 2.5, price_out = 15, price_cache_write = 3.125, price_cache_read = 0.25 WHERE id = 'gpt-5.4';
UPDATE models SET price_in = 5, price_out = 30, price_cache_write = 6.25, price_cache_read = 0.5 WHERE id = 'gpt-5.5';
UPDATE models SET price_in = 1.75, price_out = 14, price_cache_write = 2.1875, price_cache_read = 0.175 WHERE id IN ('gpt-5.2', 'gpt-5.3-codex');
