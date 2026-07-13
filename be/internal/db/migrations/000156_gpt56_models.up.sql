-- Seed the GPT-5.6 codex models (sol/terra/luna, shipped in codex CLI 0.144.0)
-- and re-point agent definitions from GPT-5.5 to GPT-5.6. The 5.5 rows stay as
-- selectable built-ins (mirrors the 5.4 -> 5.5 precedent in 000144).
--
-- gpt-5.6-sol is the new frontier/default model, gpt-5.6-terra the balanced
-- everyday model, gpt-5.6-luna the fast/affordable one. All three have a 372k
-- context window per the codex 0.144.1 bundled model catalog.

INSERT OR IGNORE INTO cli_models (id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at) VALUES
('codex_gpt56_sol_normal',   'codex', 'Codex GPT-5.6 Sol (Normal)',   'gpt-5.6-sol',   'medium', 372000, 1, 1, '2026-07-13T00:00:00Z', '2026-07-13T00:00:00Z'),
('codex_gpt56_sol_high',     'codex', 'Codex GPT-5.6 Sol (High)',     'gpt-5.6-sol',   'high',   372000, 1, 1, '2026-07-13T00:00:00Z', '2026-07-13T00:00:00Z'),
('codex_gpt56_terra_normal', 'codex', 'Codex GPT-5.6 Terra (Normal)', 'gpt-5.6-terra', 'medium', 372000, 1, 1, '2026-07-13T00:00:00Z', '2026-07-13T00:00:00Z'),
('codex_gpt56_terra_high',   'codex', 'Codex GPT-5.6 Terra (High)',   'gpt-5.6-terra', 'high',   372000, 1, 1, '2026-07-13T00:00:00Z', '2026-07-13T00:00:00Z'),
('codex_gpt56_luna_low',     'codex', 'Codex GPT-5.6 Luna (Low)',     'gpt-5.6-luna',  'low',    372000, 1, 1, '2026-07-13T00:00:00Z', '2026-07-13T00:00:00Z');

-- OpenAI API models (mirror the gpt55_* set from 000144; sol is the mainline).
-- The codex 0.144 catalog marks the 5.6 family supported_in_api.
INSERT OR IGNORE INTO api_models (id, provider, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at) VALUES
('gpt56_sol_high',   'openai', 'GPT-5.6 Sol (High)',   'gpt-5.6-sol', 'high',   372000, 1, 1, '2026-07-13T00:00:00Z', '2026-07-13T00:00:00Z'),
('gpt56_sol_low',    'openai', 'GPT-5.6 Sol (Low)',    'gpt-5.6-sol', 'low',    372000, 1, 1, '2026-07-13T00:00:00Z', '2026-07-13T00:00:00Z'),
('gpt56_sol_medium', 'openai', 'GPT-5.6 Sol (Medium)', 'gpt-5.6-sol', 'medium', 372000, 1, 1, '2026-07-13T00:00:00Z', '2026-07-13T00:00:00Z');

-- Re-point agent + system agent definitions from ALL older codex generations
-- to gpt-5.6: gpt-5.3-codex (codex_gpt_*, deprecated upstream 2026-05-26),
-- gpt-5.4 (codex_gpt54_*), and gpt-5.5 (codex_gpt55_*). Sol (frontier, picker
-- priority 1) succeeds the mainline tiers; Luna succeeds the mini tier.
UPDATE agent_definitions        SET model                 = 'codex_gpt56_sol_normal' WHERE model                 IN ('codex_gpt_normal', 'codex_gpt54_normal', 'codex_gpt55_normal');
UPDATE agent_definitions        SET model                 = 'codex_gpt56_sol_high'   WHERE model                 IN ('codex_gpt_high', 'codex_gpt54_high', 'codex_gpt55_high');
UPDATE agent_definitions        SET model                 = 'codex_gpt56_luna_low'   WHERE model                 IN ('codex_gpt54_mini_low', 'codex_gpt55_mini_low');
UPDATE agent_definitions        SET low_consumption_model = 'codex_gpt56_sol_normal' WHERE low_consumption_model IN ('codex_gpt_normal', 'codex_gpt54_normal', 'codex_gpt55_normal');
UPDATE agent_definitions        SET low_consumption_model = 'codex_gpt56_sol_high'   WHERE low_consumption_model IN ('codex_gpt_high', 'codex_gpt54_high', 'codex_gpt55_high');
UPDATE agent_definitions        SET low_consumption_model = 'codex_gpt56_luna_low'   WHERE low_consumption_model IN ('codex_gpt54_mini_low', 'codex_gpt55_mini_low');
UPDATE system_agent_definitions SET model                 = 'codex_gpt56_sol_normal' WHERE model                 IN ('codex_gpt_normal', 'codex_gpt54_normal', 'codex_gpt55_normal');
UPDATE system_agent_definitions SET model                 = 'codex_gpt56_sol_high'   WHERE model                 IN ('codex_gpt_high', 'codex_gpt54_high', 'codex_gpt55_high');
UPDATE system_agent_definitions SET model                 = 'codex_gpt56_luna_low'   WHERE model                 IN ('codex_gpt54_mini_low', 'codex_gpt55_mini_low');
