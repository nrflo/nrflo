-- Claude Opus 5 (`claude-opus-5`): 1M API context, 128k max output, $5/$25 per
-- MTok, efforts low..max (provider default `high`). Mirrors the opus-4-8 pair:
-- the bare CLI string opens 200k and the "[1m]" suffix opens 1M, so the 1M row
-- carries the bare id as its --fallback-model chain.
INSERT INTO models (
    id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
    cli_context, api_context, fallback_models, default_effort, read_only,
    enabled, created_at, updated_at, release_date,
    price_in, price_out, price_cache_write, price_cache_read
) VALUES
('opus-5', 'anthropic', 'Claude Opus 5', 'claude-opus-5', 'claude-opus-5', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 200000, 1000000, '', '', 1, 1, '2026-07-25T00:00:00Z', '2026-07-25T00:00:00Z', '2026-07-24', 5, 25, 6.25, 0.5),
('opus-5-1m', 'anthropic', 'Claude Opus 5 (1M)', 'claude-opus-5[1m]', 'claude-opus-5[1m]', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 1000000, 1000000, 'claude-opus-5', '', 1, 1, '2026-07-25T00:00:00Z', '2026-07-25T00:00:00Z', '2026-07-24', 5, 25, 6.25, 0.5);

-- Fable 5's overload fallback moves to the current Opus.
UPDATE models SET fallback_models = 'claude-opus-5', updated_at = '2026-07-25T00:00:00Z'
WHERE id = 'fable-5';

-- Migrate references opus-4-8 -> opus-5. The 4.8 catalog rows stay enabled:
-- the provider still serves them and historical sessions must resolve. Follows
-- 000170: agent_definitions / system_agent_definitions / workflows / tier_models
-- only, never agent_sessions (a finished session records what actually ran).
CREATE TEMP TABLE opus5_map (
    old_id TEXT PRIMARY KEY,
    new_id TEXT NOT NULL
);

INSERT INTO opus5_map (old_id, new_id) VALUES
('opus-4-8', 'opus-5'),
('opus-4-8-1m', 'opus-5-1m');

UPDATE agent_definitions
SET model = (SELECT new_id FROM opus5_map WHERE old_id = model)
WHERE model IN (SELECT old_id FROM opus5_map);
UPDATE agent_definitions
SET low_consumption_model = (SELECT new_id FROM opus5_map WHERE old_id = low_consumption_model)
WHERE low_consumption_model IN (SELECT old_id FROM opus5_map);

UPDATE system_agent_definitions
SET model = (SELECT new_id FROM opus5_map WHERE old_id = model)
WHERE model IN (SELECT old_id FROM opus5_map);

UPDATE workflows
SET observer_model = (SELECT new_id FROM opus5_map WHERE old_id = observer_model)
WHERE observer_model IN (SELECT old_id FROM opus5_map);

UPDATE tier_models
SET model_id = (SELECT new_id FROM opus5_map WHERE old_id = model_id)
WHERE model_id IN (SELECT old_id FROM opus5_map);

DROP TABLE opus5_map;
