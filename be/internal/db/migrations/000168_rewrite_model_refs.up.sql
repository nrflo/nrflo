CREATE TEMP TABLE model_id_map (
    old_id TEXT PRIMARY KEY,
    new_id TEXT NOT NULL,
    effort TEXT
);

INSERT INTO model_id_map (old_id, new_id, effort) VALUES
('sonnet', 'sonnet-5', NULL),
('haiku', 'haiku-4-5', NULL),
('opus_4_6', 'opus-4-6', NULL),
('opus_4_6_1m', 'opus-4-6-1m', NULL),
('opus_4_7', 'opus-4-7', NULL),
('opus_4_7_1m', 'opus-4-7-1m', NULL),
('opus_4_8', 'opus-4-8', NULL),
('opus_4_8_1m', 'opus-4-8-1m', NULL),
('codex_gpt_normal', 'gpt-5.3-codex', 'high'),
('codex_gpt_high', 'gpt-5.3-codex', 'high'),
('gpt53_codex_low', 'gpt-5.3-codex', 'low'),
('gpt53_codex_medium', 'gpt-5.3-codex', 'medium'),
('gpt53_codex_high', 'gpt-5.3-codex', 'high'),
('codex_gpt54_normal', 'gpt-5.4', 'medium'),
('codex_gpt54_high', 'gpt-5.4', 'high'),
('gpt54_low', 'gpt-5.4', 'low'),
('gpt54_medium', 'gpt-5.4', 'medium'),
('gpt54_high', 'gpt-5.4', 'high'),
('codex_gpt54_mini_low', 'gpt-5.4-mini', 'low'),
('codex_gpt55_normal', 'gpt-5.5', 'medium'),
('codex_gpt55_high', 'gpt-5.5', 'high'),
('gpt55_low', 'gpt-5.5', 'low'),
('gpt55_medium', 'gpt-5.5', 'medium'),
('gpt55_high', 'gpt-5.5', 'high'),
('codex_gpt55_mini_low', 'gpt-5.5-mini', 'low'),
('codex_gpt56_sol_normal', 'gpt-5.6-sol', 'medium'),
('codex_gpt56_sol_high', 'gpt-5.6-sol', 'high'),
('gpt56_sol_low', 'gpt-5.6-sol', 'low'),
('gpt56_sol_medium', 'gpt-5.6-sol', 'medium'),
('gpt56_sol_high', 'gpt-5.6-sol', 'high'),
('codex_gpt56_terra_normal', 'gpt-5.6-terra', 'medium'),
('codex_gpt56_terra_high', 'gpt-5.6-terra', 'high'),
('codex_gpt56_luna_low', 'gpt-5.6-luna', 'low');

UPDATE agent_definitions
SET reasoning_effort = (SELECT effort FROM model_id_map WHERE old_id = model)
WHERE reasoning_effort IS NULL
  AND model IN (SELECT old_id FROM model_id_map WHERE effort IS NOT NULL);
UPDATE agent_definitions
SET model = (SELECT new_id FROM model_id_map WHERE old_id = model)
WHERE model IN (SELECT old_id FROM model_id_map);
UPDATE agent_definitions
SET low_consumption_model = (SELECT new_id FROM model_id_map WHERE old_id = low_consumption_model)
WHERE low_consumption_model IN (SELECT old_id FROM model_id_map);

UPDATE system_agent_definitions
SET reasoning_effort = (SELECT effort FROM model_id_map WHERE old_id = model)
WHERE reasoning_effort IS NULL
  AND model IN (SELECT old_id FROM model_id_map WHERE effort IS NOT NULL);
UPDATE system_agent_definitions
SET model = (SELECT new_id FROM model_id_map WHERE old_id = model)
WHERE model IN (SELECT old_id FROM model_id_map);

UPDATE agent_sessions
SET model_id = (SELECT new_id FROM model_id_map WHERE old_id = model_id)
WHERE model_id IN (SELECT old_id FROM model_id_map);

DROP TABLE model_id_map;
