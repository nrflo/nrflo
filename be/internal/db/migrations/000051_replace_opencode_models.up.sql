-- Remove old opencode models
DELETE FROM cli_models WHERE id IN ('opencode_gpt_normal', 'opencode_gpt_high');

-- Insert three new opencode models
INSERT OR IGNORE INTO cli_models (id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, created_at, updated_at) VALUES
('opencode_minimax_m25_free', 'opencode', 'OpenCode Minimax M2.5 Free', 'opencode/minimax-m2.5-free', '', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('opencode_qwen36_plus_free', 'opencode', 'OpenCode Qwen 3.6 Plus Free', 'opencode/qwen3.6-plus-free', '', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('opencode_gpt54', 'opencode', 'OpenCode GPT 5.4', 'openai/gpt-5.4', 'high', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');

-- Migrate agent_definitions referencing old models
UPDATE agent_definitions SET model = 'opencode_gpt54' WHERE model IN ('opencode_gpt_normal', 'opencode_gpt_high');
UPDATE agent_definitions SET low_consumption_model = 'opencode_gpt54' WHERE low_consumption_model IN ('opencode_gpt_normal', 'opencode_gpt_high');

-- Migrate system_agent_definitions referencing old models
UPDATE system_agent_definitions SET model = 'opencode_gpt54' WHERE model IN ('opencode_gpt_normal', 'opencode_gpt_high');
