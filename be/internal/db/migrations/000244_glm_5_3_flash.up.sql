-- Ox Alpha was the free stealth preview of GLM 5.3 Flash. Replace the
-- temporary identity with the released OpenRouter model and route tiers 1-4
-- through it first; subscription-backed Claude/Codex entries remain ordered
-- fallbacks. Tier 5 stays premium-only.
INSERT INTO models (
    id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
    cli_context, api_context, fallback_models, default_effort, read_only,
    enabled, created_at, updated_at, release_date,
    price_in, price_out, price_cache_write, price_cache_read
) VALUES (
    'glm-5.3-flash', 'openrouter', 'GLM 5.3 Flash (OpenRouter)', '',
    'z-ai/glm-5.3-flash', '[]', '["low","high","max"]', 1048576,
    1048576, '', 'high', 1, 1, '2026-08-27T00:00:00Z',
    '2026-08-27T00:00:00Z', '2026-08-27', 0.075, 0.25, 0.275, 0.015
)
ON CONFLICT(id) DO UPDATE SET
    provider = excluded.provider,
    display_name = excluded.display_name,
    cli_model = excluded.cli_model,
    api_model = excluded.api_model,
    cli_efforts = excluded.cli_efforts,
    api_efforts = excluded.api_efforts,
    cli_context = excluded.cli_context,
    api_context = excluded.api_context,
    fallback_models = excluded.fallback_models,
    default_effort = excluded.default_effort,
    read_only = excluded.read_only,
    enabled = excluded.enabled,
    updated_at = excluded.updated_at,
    release_date = excluded.release_date,
    price_in = excluded.price_in,
    price_out = excluded.price_out,
    price_cache_write = excluded.price_cache_write,
    price_cache_read = excluded.price_cache_read;

UPDATE agent_definitions SET model = 'glm-5.3-flash' WHERE model = 'ox-alpha';
UPDATE agent_definitions SET low_consumption_model = 'glm-5.3-flash'
WHERE low_consumption_model = 'ox-alpha';
UPDATE system_agent_definitions SET model = 'glm-5.3-flash'
WHERE model = 'ox-alpha';
UPDATE workflows SET observer_model = 'glm-5.3-flash'
WHERE observer_model = 'ox-alpha';
UPDATE config SET value = 'glm-5.3-flash'
WHERE key = 'observer_model' AND value = 'ox-alpha';
UPDATE agent_sessions SET model_id = 'glm-5.3-flash'
WHERE model_id = 'ox-alpha';

CREATE TEMP TABLE glm_previous_tier_models AS
SELECT tier, position, provider, execution_mode, model_id, reasoning_effort,
       weight
FROM tier_models
WHERE tier BETWEEN 1 AND 5
  AND model_id NOT IN ('ox-alpha', 'glm-5.3-flash');

DELETE FROM tier_models WHERE tier BETWEEN 1 AND 5;

INSERT INTO tier_models (
    tier, position, provider, execution_mode, model_id, reasoning_effort, weight
) VALUES
    (1, 0, 'openrouter', 'api', 'glm-5.3-flash', 'low', 0),
    (2, 0, 'openrouter', 'api', 'glm-5.3-flash', 'high', 0),
    (3, 0, 'openrouter', 'api', 'glm-5.3-flash', 'max', 0),
    (4, 0, 'openrouter', 'api', 'glm-5.3-flash', 'max', 0);

INSERT INTO tier_models (
    tier, position, provider, execution_mode, model_id, reasoning_effort, weight
)
SELECT tier,
       ROW_NUMBER() OVER (PARTITION BY tier ORDER BY position) -
           CASE WHEN tier BETWEEN 1 AND 4 THEN 0 ELSE 1 END,
       provider, execution_mode, model_id, reasoning_effort, weight
FROM glm_previous_tier_models
ORDER BY tier, position;

DROP TABLE glm_previous_tier_models;
DELETE FROM models WHERE id = 'ox-alpha';
