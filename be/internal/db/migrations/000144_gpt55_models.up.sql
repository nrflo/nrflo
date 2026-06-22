-- Seed GPT-5.5 models for the GPT providers and re-point agent definitions
-- from GPT-5.4 to GPT-5.5. The 5.4 rows are intentionally kept as selectable
-- built-ins (mirrors the opus 4.7 -> 4.8 precedent in 000138).

-- codex CLI models (mirror the codex_gpt54_* set).
INSERT OR IGNORE INTO cli_models (id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at) VALUES
('codex_gpt55_normal',   'codex', 'Codex GPT-55 (Normal)',   'gpt-5.5',      'medium', 200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('codex_gpt55_high',     'codex', 'Codex GPT-55 (High)',     'gpt-5.5',      'high',   200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('codex_gpt55_mini_low', 'codex', 'Codex GPT-55 Mini (Low)', 'gpt-5.5-mini', 'low',    200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');

-- OpenAI API models (mirror the gpt54_* set).
INSERT OR IGNORE INTO api_models (id, provider, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at) VALUES
('gpt55_high',   'openai', 'GPT-5.5 (High)',   'gpt-5.5', 'high',   200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('gpt55_low',    'openai', 'GPT-5.5 (Low)',    'gpt-5.5', 'low',    200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('gpt55_medium', 'openai', 'GPT-5.5 (Medium)', 'gpt-5.5', 'medium', 200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');

-- Re-point agent + system agent definitions from the gpt-5.4 ids to gpt-5.5.
UPDATE agent_definitions        SET model                 = 'codex_gpt55_normal'   WHERE model                 = 'codex_gpt54_normal';
UPDATE agent_definitions        SET model                 = 'codex_gpt55_high'     WHERE model                 = 'codex_gpt54_high';
UPDATE agent_definitions        SET model                 = 'codex_gpt55_mini_low' WHERE model                 = 'codex_gpt54_mini_low';
UPDATE agent_definitions        SET low_consumption_model = 'codex_gpt55_normal'   WHERE low_consumption_model = 'codex_gpt54_normal';
UPDATE agent_definitions        SET low_consumption_model = 'codex_gpt55_high'     WHERE low_consumption_model = 'codex_gpt54_high';
UPDATE agent_definitions        SET low_consumption_model = 'codex_gpt55_mini_low' WHERE low_consumption_model = 'codex_gpt54_mini_low';
UPDATE system_agent_definitions SET model                 = 'codex_gpt55_normal'   WHERE model                 = 'codex_gpt54_normal';
UPDATE system_agent_definitions SET model                 = 'codex_gpt55_high'     WHERE model                 = 'codex_gpt54_high';
UPDATE system_agent_definitions SET model                 = 'codex_gpt55_mini_low' WHERE model                 = 'codex_gpt54_mini_low';
