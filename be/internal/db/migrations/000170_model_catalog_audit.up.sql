INSERT INTO models (
    id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
    cli_context, api_context, fallback_models, default_effort, read_only,
    enabled, created_at, updated_at
) VALUES
('fable-5', 'anthropic', 'Claude Fable 5', 'claude-fable-5', 'claude-fable-5', '["low","medium","high","xhigh","max"]', '["low","medium","high","xhigh","max"]', 1000000, 1000000, 'claude-opus-4-8', '', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z'),
('gpt-5.2', 'openai', 'GPT-5.2', 'gpt-5.2', 'gpt-5.2', '["low","medium","high","xhigh"]', '["low","medium","high","xhigh"]', 200000, 400000, '', 'medium', 1, 1, '2026-07-16T00:00:00Z', '2026-07-16T00:00:00Z');

-- Codex no longer advertises GPT-5.3 Codex or GPT-5.5 Mini. Keep the former
-- for API mode, and keep the latter disabled so historical sessions still
-- resolve to a catalog row.
UPDATE models SET
    cli_model = '', cli_efforts = '[]', api_context = 400000,
    updated_at = '2026-07-16T00:00:00Z'
WHERE id = 'gpt-5.3-codex';
UPDATE models SET enabled = 0, updated_at = '2026-07-16T00:00:00Z'
WHERE id = 'gpt-5.5-mini';

-- OpenAI's current API catalog exposes these models and context windows.
UPDATE models SET
    api_model = 'gpt-5.4-mini', api_efforts = '["low","medium","high","xhigh"]',
    api_context = 400000, default_effort = 'medium',
    updated_at = '2026-07-16T00:00:00Z'
WHERE id = 'gpt-5.4-mini';
UPDATE models SET api_context = 1050000, updated_at = '2026-07-16T00:00:00Z'
WHERE id IN ('gpt-5.4', 'gpt-5.5', 'gpt-5.6-sol');
UPDATE models SET
    api_model = id, api_efforts = '["low","medium","high","xhigh","max"]',
    api_context = 1050000, updated_at = '2026-07-16T00:00:00Z'
WHERE id IN ('gpt-5.6-terra', 'gpt-5.6-luna');

-- Match the defaults currently reported by Codex app-server.
UPDATE models SET default_effort = 'low', updated_at = '2026-07-16T00:00:00Z'
WHERE id = 'gpt-5.6-sol';
UPDATE models SET default_effort = 'medium', updated_at = '2026-07-16T00:00:00Z'
WHERE id = 'gpt-5.6-luna';

-- Preserve the old inherited effort before replacing a CLI-only model whose
-- replacement has a different default.
UPDATE agent_definitions SET reasoning_effort = 'high'
WHERE model = 'gpt-5.3-codex' AND execution_mode = 'cli_interactive'
  AND reasoning_effort IS NULL;
UPDATE system_agent_definitions SET reasoning_effort = 'high'
WHERE model = 'gpt-5.3-codex' AND execution_mode = 'cli_interactive'
  AND reasoning_effort IS NULL;
UPDATE agent_definitions SET reasoning_effort = 'low'
WHERE model = 'gpt-5.5-mini' AND reasoning_effort IS NULL;
UPDATE system_agent_definitions SET reasoning_effort = 'low'
WHERE model = 'gpt-5.5-mini' AND reasoning_effort IS NULL;

UPDATE agent_definitions SET model = 'gpt-5.2'
WHERE model = 'gpt-5.3-codex' AND execution_mode = 'cli_interactive';
UPDATE agent_definitions SET low_consumption_model = 'gpt-5.2'
WHERE low_consumption_model = 'gpt-5.3-codex' AND execution_mode = 'cli_interactive';
UPDATE system_agent_definitions SET model = 'gpt-5.2'
WHERE model = 'gpt-5.3-codex' AND execution_mode = 'cli_interactive';

UPDATE agent_definitions SET model = 'gpt-5.6-luna'
WHERE model = 'gpt-5.5-mini';
UPDATE agent_definitions SET low_consumption_model = 'gpt-5.6-luna'
WHERE low_consumption_model = 'gpt-5.5-mini';
UPDATE system_agent_definitions SET model = 'gpt-5.6-luna'
WHERE model = 'gpt-5.5-mini';

UPDATE workflows SET observer_model = 'gpt-5.2'
WHERE observer_model = 'gpt-5.3-codex' AND observer_provider = 'codex';
UPDATE workflows SET observer_model = 'gpt-5.6-luna'
WHERE observer_model = 'gpt-5.5-mini';
