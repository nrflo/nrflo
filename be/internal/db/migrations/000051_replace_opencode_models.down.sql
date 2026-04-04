-- Remove new opencode models
DELETE FROM cli_models WHERE id IN ('opencode_minimax_m25_free', 'opencode_qwen36_plus_free', 'opencode_gpt54');

-- Re-insert old opencode models
INSERT OR IGNORE INTO cli_models (id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, created_at, updated_at) VALUES
('opencode_gpt_normal', 'opencode', 'OpenCode GPT (Normal)', 'openai/gpt-5.3-codex', 'high', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('opencode_gpt_high', 'opencode', 'OpenCode GPT (High)', 'openai/gpt-5.3-codex', 'high', 200000, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');

-- Best-effort rollback: set back to opencode_gpt_high (cannot know original per-row values)
UPDATE agent_definitions SET model = 'opencode_gpt_high' WHERE model = 'opencode_gpt54';
UPDATE agent_definitions SET low_consumption_model = 'opencode_gpt_high' WHERE low_consumption_model = 'opencode_gpt54';
UPDATE system_agent_definitions SET model = 'opencode_gpt_high' WHERE model = 'opencode_gpt54';
