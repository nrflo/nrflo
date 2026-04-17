-- Seed versioned Opus models (read_only = 1, enabled = 1)
INSERT OR IGNORE INTO cli_models (id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at) VALUES
('opus_4_6', 'claude', 'Opus 4.6', 'claude-opus-4-6', '', 200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('opus_4_6_1m', 'claude', 'Opus 4.6 (1M)', 'claude-opus-4-6[1m]', '', 1000000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('opus_4_7', 'claude', 'Opus 4.7', 'claude-opus-4-7', '', 200000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
('opus_4_7_1m', 'claude', 'Opus 4.7 (1M)', 'claude-opus-4-7[1m]', '', 1000000, 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');

-- Migrate agent_definitions referencing old opus/opus_1m to opus_4_7/opus_4_7_1m
UPDATE agent_definitions SET model = 'opus_4_7' WHERE model = 'opus';
UPDATE agent_definitions SET model = 'opus_4_7_1m' WHERE model = 'opus_1m';
UPDATE agent_definitions SET low_consumption_model = 'opus_4_7' WHERE low_consumption_model = 'opus';
UPDATE agent_definitions SET low_consumption_model = 'opus_4_7_1m' WHERE low_consumption_model = 'opus_1m';

-- Migrate system_agent_definitions referencing old opus/opus_1m
UPDATE system_agent_definitions SET model = 'opus_4_7' WHERE model = 'opus';
UPDATE system_agent_definitions SET model = 'opus_4_7_1m' WHERE model = 'opus_1m';

-- Remove the old unversioned opus rows after migration
DELETE FROM cli_models WHERE id IN ('opus', 'opus_1m');
