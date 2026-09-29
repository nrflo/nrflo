-- Claude Sonnet 5.5 (2026-09-28): $2/$10, cache $2.50 write / $0.20 read,
-- 1M context native (no "[1m]" row, like sonnet-5), efforts low..max.
INSERT INTO models (
    id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
    cli_context, api_context, fallback_models, default_effort, read_only,
    enabled, created_at, updated_at, release_date,
    price_in, price_out, price_cache_write, price_cache_read
) VALUES
('sonnet-5-5', 'anthropic', 'Claude Sonnet 5.5', 'claude-sonnet-5-5', 'claude-sonnet-5-5', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 1000000, 1000000, '', '', 1, 1, '2026-09-29T00:00:00Z', '2026-09-29T00:00:00Z', '2026-09-28', 2, 10, 2.5, 0.2);

-- Migrate references; the sonnet-5 row stays enabled and agent_sessions is
-- never rewritten (000170/000210).
UPDATE agent_definitions SET model = 'sonnet-5-5' WHERE model = 'sonnet-5';
UPDATE agent_definitions SET low_consumption_model = 'sonnet-5-5' WHERE low_consumption_model = 'sonnet-5';
UPDATE system_agent_definitions SET model = 'sonnet-5-5' WHERE model = 'sonnet-5';
UPDATE workflows SET observer_model = 'sonnet-5-5' WHERE observer_model = 'sonnet-5';
UPDATE config SET value = 'sonnet-5-5' WHERE key = 'observer_model' AND value = 'sonnet-5';
UPDATE tier_models SET model_id = 'sonnet-5-5' WHERE model_id = 'sonnet-5';
