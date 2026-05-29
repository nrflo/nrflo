-- Seed Opus 4.8 CLI models (read_only = 1, enabled = 1)
INSERT OR IGNORE INTO cli_models (id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at) VALUES
('opus_4_8', 'claude', 'Opus 4.8', 'claude-opus-4-8', '', 200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('opus_4_8_1m', 'claude', 'Opus 4.8 (1M)', 'claude-opus-4-8[1m]', '', 1000000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');

-- Seed Opus 4.8 API models (anthropic, read_only = 1, enabled = 1)
INSERT OR IGNORE INTO api_models (id, provider, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at) VALUES
('opus_4_8', 'anthropic', 'Claude Opus 4.8', 'claude-opus-4-8', '', 200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('opus_4_8_1m', 'anthropic', 'Claude Opus 4.8 (1M)', 'claude-opus-4-8[1m]', '', 1000000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');

-- Migrate agent_definitions referencing opus_4_7/opus_4_7_1m to opus_4_8/opus_4_8_1m
UPDATE agent_definitions SET model = 'opus_4_8' WHERE model = 'opus_4_7';
UPDATE agent_definitions SET model = 'opus_4_8_1m' WHERE model = 'opus_4_7_1m';
UPDATE agent_definitions SET low_consumption_model = 'opus_4_8' WHERE low_consumption_model = 'opus_4_7';
UPDATE agent_definitions SET low_consumption_model = 'opus_4_8_1m' WHERE low_consumption_model = 'opus_4_7_1m';

-- Migrate system_agent_definitions referencing opus_4_7/opus_4_7_1m
UPDATE system_agent_definitions SET model = 'opus_4_8' WHERE model = 'opus_4_7';
UPDATE system_agent_definitions SET model = 'opus_4_8_1m' WHERE model = 'opus_4_7_1m';

-- Opus 4.7 rows are intentionally NOT deleted; they remain selectable built-ins.
