-- Cross-provider codex hops for the sonnet tiers, mirroring tier 1's luna
-- hop (000220): an Anthropic outage/rate-limit must not exhaust a chain that
-- only ever names anthropic. gpt-5.6-terra is the sonnet price-class match
-- ($2.5/$15 vs $3/$15); cli_interactive rides codex CLI OAuth, so no
-- OPENAI_API_KEY is required. Guarded per tier: only appended while the
-- chain is still the seeded two-entry sonnet default (000200/000195) and the
-- terra row is usable; customized chains are left untouched.

INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort)
SELECT 2, 2, 'openai', 'cli_interactive', 'gpt-5.6-terra', 'low'
WHERE (SELECT COUNT(*) FROM tier_models WHERE tier = 2) = 2
  AND (SELECT COUNT(*) FROM tier_models WHERE tier = 2 AND model_id = 'sonnet-5') = 2
  AND EXISTS (SELECT 1 FROM models WHERE id = 'gpt-5.6-terra' AND enabled = 1);

INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort)
SELECT 3, 2, 'openai', 'cli_interactive', 'gpt-5.6-terra', 'medium'
WHERE (SELECT COUNT(*) FROM tier_models WHERE tier = 3) = 2
  AND (SELECT COUNT(*) FROM tier_models WHERE tier = 3 AND model_id = 'sonnet-5') = 2
  AND EXISTS (SELECT 1 FROM models WHERE id = 'gpt-5.6-terra' AND enabled = 1);

INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort)
SELECT 4, 2, 'openai', 'cli_interactive', 'gpt-5.6-terra', 'medium'
WHERE (SELECT COUNT(*) FROM tier_models WHERE tier = 4) = 2
  AND (SELECT COUNT(*) FROM tier_models WHERE tier = 4 AND model_id = 'sonnet-5') = 2
  AND EXISTS (SELECT 1 FROM models WHERE id = 'gpt-5.6-terra' AND enabled = 1);

-- Seed tier 5 (premium): reserved until now — the API/tier UI ranges over
-- 1-5 but nothing populated 5, so premium defs had to pin models. Same
-- shape as the other tiers: anthropic api -> cli, then the codex hop
-- (gpt-5.6-sol is the opus price class).
INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort)
SELECT 5, 0, 'anthropic', '', 'opus-5', 'high'
WHERE (SELECT COUNT(*) FROM tier_models WHERE tier = 5) = 0
  AND EXISTS (SELECT 1 FROM models WHERE id = 'opus-5' AND enabled = 1);
INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort)
SELECT 5, 1, 'anthropic', 'cli_interactive', 'opus-5', 'high'
WHERE (SELECT COUNT(*) FROM tier_models WHERE tier = 5) = 1
  AND EXISTS (SELECT 1 FROM models WHERE id = 'opus-5' AND enabled = 1);
INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort)
SELECT 5, 2, 'openai', 'cli_interactive', 'gpt-5.6-sol', 'high'
WHERE (SELECT COUNT(*) FROM tier_models WHERE tier = 5) = 2
  AND EXISTS (SELECT 1 FROM models WHERE id = 'gpt-5.6-sol' AND enabled = 1);

-- Make the chains effective: a pinned model is a single-entry chain, so the
-- hops above would never fire for a pinned def. Clear each pin only while it
-- still equals its tier chain's head (same primary model/effort, fallback
-- gained — behavior-preserving); a customized pin is respected.
UPDATE system_agent_definitions SET model = '', updated_at = datetime('now')
WHERE (id IN ('_t1_executor', 'planner-system', 'planner-system-api', 'conflict-resolver') AND model = 'sonnet-5')
   OR (id IN ('spec-normalizer', 'context-saver-api', 'context-saver') AND model = 'haiku-4-5');
