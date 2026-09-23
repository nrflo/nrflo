-- Claude Opus 5.5 (2026-09-22): $4/$20, cache $5 write / $0.20 read, efforts
-- low..max (provider default medium). Mirrors the opus-5 pair: the bare CLI
-- string opens 200k, the "[1m]" suffix opens 1M. Claude Fable 5.1
-- (2026-09-01): $10/$50, cache read $0.25, 1M native like fable-5.
-- GPT-6 (2026-09-22) from codex app-server model/list + OpenAI pricing:
-- astra $10/$50 frontier, sol $2/$10 workhorse, luna $0.10/$0.50 fast; codex
-- context 272k, API 1.05M; ultra is codex-only on astra/sol. OpenAI rows keep
-- the 000183 convention cache_write = 1.25x price_in.
INSERT INTO models (
    id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
    cli_context, api_context, fallback_models, default_effort, read_only,
    enabled, created_at, updated_at, release_date,
    price_in, price_out, price_cache_write, price_cache_read
) VALUES
('opus-5-5', 'anthropic', 'Claude Opus 5.5', 'claude-opus-5-5', 'claude-opus-5-5', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 200000, 1000000, '', '', 1, 1, '2026-09-23T00:00:00Z', '2026-09-23T00:00:00Z', '2026-09-22', 4, 20, 5, 0.2),
('opus-5-5-1m', 'anthropic', 'Claude Opus 5.5 (1M)', 'claude-opus-5-5[1m]', 'claude-opus-5-5[1m]', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 1000000, 1000000, 'claude-opus-5-5', '', 1, 1, '2026-09-23T00:00:00Z', '2026-09-23T00:00:00Z', '2026-09-22', 4, 20, 5, 0.2),
('fable-5-1', 'anthropic', 'Claude Fable 5.1', 'claude-fable-5-1', 'claude-fable-5-1', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 1000000, 1000000, 'claude-opus-5-5', '', 1, 1, '2026-09-23T00:00:00Z', '2026-09-23T00:00:00Z', '2026-09-01', 10, 50, 12.5, 0.25),
('gpt-6-astra', 'openai', 'GPT-6 Astra', 'gpt-6-astra', 'gpt-6-astra', '["low","medium","high","xhigh","max","ultra"]', '["low","medium","high","xhigh","max"]', 272000, 1050000, '', 'low', 1, 1, '2026-09-23T00:00:00Z', '2026-09-23T00:00:00Z', '2026-09-22', 10, 50, 12.5, 1),
('gpt-6-sol', 'openai', 'GPT-6 Sol', 'gpt-6-sol', 'gpt-6-sol', '["low","medium","high","xhigh","max","ultra"]', '["low","medium","high","xhigh","max"]', 272000, 1050000, '', 'medium', 1, 1, '2026-09-23T00:00:00Z', '2026-09-23T00:00:00Z', '2026-09-22', 2, 10, 2.5, 0.2),
('gpt-6-luna', 'openai', 'GPT-6 Luna', 'gpt-6-luna', 'gpt-6-luna', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 272000, 1050000, '', 'medium', 1, 1, '2026-09-23T00:00:00Z', '2026-09-23T00:00:00Z', '2026-09-22', 0.1, 0.5, 0.125, 0.01);

-- Fable 5's overload fallback moves to the current Opus.
UPDATE models SET fallback_models = 'claude-opus-5-5', updated_at = '2026-09-23T00:00:00Z'
WHERE id = 'fable-5';

-- OpenAI cut GPT-5.6 short-context pricing alongside the GPT-6 launch.
UPDATE models SET price_in = 4, price_out = 20, price_cache_write = 5, price_cache_read = 0.4, updated_at = '2026-09-23T00:00:00Z'
WHERE id = 'gpt-5.6-sol';
UPDATE models SET price_in = 2, price_out = 12, price_cache_write = 2.5, price_cache_read = 0.2, updated_at = '2026-09-23T00:00:00Z'
WHERE id = 'gpt-5.6-terra';
UPDATE models SET price_in = 0.2, price_out = 1.2, price_cache_write = 0.25, price_cache_read = 0.02, updated_at = '2026-09-23T00:00:00Z'
WHERE id = 'gpt-5.6-luna';

-- Migrate references to the new generation. Old catalog rows stay enabled
-- (still served; historical sessions must resolve). Per 000170/000210,
-- agent_sessions is never rewritten. GPT-5.6 sol and terra both collapse onto
-- GPT-6 Sol (the $2/$10 workhorse); astra is selectable but not wired in.
CREATE TEMP TABLE gen_map (
    old_id TEXT PRIMARY KEY,
    new_id TEXT NOT NULL
);

INSERT INTO gen_map (old_id, new_id) VALUES
('opus-5', 'opus-5-5'),
('opus-5-1m', 'opus-5-5-1m'),
('fable-5', 'fable-5-1'),
('gpt-5.6-sol', 'gpt-6-sol'),
('gpt-5.6-terra', 'gpt-6-sol'),
('gpt-5.6-luna', 'gpt-6-luna');

UPDATE agent_definitions
SET model = (SELECT new_id FROM gen_map WHERE old_id = model)
WHERE model IN (SELECT old_id FROM gen_map);
UPDATE agent_definitions
SET low_consumption_model = (SELECT new_id FROM gen_map WHERE old_id = low_consumption_model)
WHERE low_consumption_model IN (SELECT old_id FROM gen_map);

UPDATE system_agent_definitions
SET model = (SELECT new_id FROM gen_map WHERE old_id = model)
WHERE model IN (SELECT old_id FROM gen_map);

UPDATE workflows
SET observer_model = (SELECT new_id FROM gen_map WHERE old_id = observer_model)
WHERE observer_model IN (SELECT old_id FROM gen_map);

UPDATE config
SET value = (SELECT new_id FROM gen_map WHERE old_id = value)
WHERE key = 'observer_model' AND value IN (SELECT old_id FROM gen_map);

UPDATE tier_models
SET model_id = (SELECT new_id FROM gen_map WHERE old_id = model_id)
WHERE model_id IN (SELECT old_id FROM gen_map);

DROP TABLE gen_map;
