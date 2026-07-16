-- Migration 000168 rewrote agent_sessions.model_id only for BARE legacy ids,
-- but the spawner has always stored them as `<cli>:<slug>` (e.g. `claude:sonnet`,
-- `codex:codex_gpt56_sol_high`), so that UPDATE never matched a real row.
-- Remap the suffix after the first ':' using the same old->new slug map as 000168.
-- Effort is irrelevant for sessions (only agent_definitions carried effort), so it
-- is omitted here. Historical delistings handled by 000170 deliberately retained
-- session ids (see 000170: it rewrites agent_definitions/system_agent_definitions/
-- workflows only, never agent_sessions), so no delisting rewrite is applied here.
CREATE TEMP TABLE model_id_map (
    old_id TEXT PRIMARY KEY,
    new_id TEXT NOT NULL
);

INSERT INTO model_id_map (old_id, new_id) VALUES
('sonnet', 'sonnet-5'),
('haiku', 'haiku-4-5'),
('opus_4_6', 'opus-4-6'),
('opus_4_6_1m', 'opus-4-6-1m'),
('opus_4_7', 'opus-4-7'),
('opus_4_7_1m', 'opus-4-7-1m'),
('opus_4_8', 'opus-4-8'),
('opus_4_8_1m', 'opus-4-8-1m'),
('codex_gpt_normal', 'gpt-5.3-codex'),
('codex_gpt_high', 'gpt-5.3-codex'),
('gpt53_codex_low', 'gpt-5.3-codex'),
('gpt53_codex_medium', 'gpt-5.3-codex'),
('gpt53_codex_high', 'gpt-5.3-codex'),
('codex_gpt54_normal', 'gpt-5.4'),
('codex_gpt54_high', 'gpt-5.4'),
('gpt54_low', 'gpt-5.4'),
('gpt54_medium', 'gpt-5.4'),
('gpt54_high', 'gpt-5.4'),
('codex_gpt54_mini_low', 'gpt-5.4-mini'),
('codex_gpt55_normal', 'gpt-5.5'),
('codex_gpt55_high', 'gpt-5.5'),
('gpt55_low', 'gpt-5.5'),
('gpt55_medium', 'gpt-5.5'),
('gpt55_high', 'gpt-5.5'),
('codex_gpt55_mini_low', 'gpt-5.5-mini'),
('codex_gpt56_sol_normal', 'gpt-5.6-sol'),
('codex_gpt56_sol_high', 'gpt-5.6-sol'),
('gpt56_sol_low', 'gpt-5.6-sol'),
('gpt56_sol_medium', 'gpt-5.6-sol'),
('gpt56_sol_high', 'gpt-5.6-sol'),
('codex_gpt56_terra_normal', 'gpt-5.6-terra'),
('codex_gpt56_terra_high', 'gpt-5.6-terra'),
('codex_gpt56_luna_low', 'gpt-5.6-luna');

UPDATE agent_sessions
SET model_id = substr(model_id, 1, instr(model_id, ':'))
    || (SELECT new_id FROM model_id_map
        WHERE old_id = substr(agent_sessions.model_id, instr(agent_sessions.model_id, ':') + 1))
WHERE instr(model_id, ':') > 0
  AND substr(model_id, instr(model_id, ':') + 1) IN (SELECT old_id FROM model_id_map);

DROP TABLE model_id_map;
