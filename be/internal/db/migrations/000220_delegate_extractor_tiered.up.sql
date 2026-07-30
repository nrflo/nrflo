-- Delegate extractor: resolve from the tier-1 fallback chain instead of a
-- pinned model, give it native repo access, and widen the iteration budget.
--
-- 1. model='' — a pinned model is a single-entry chain (no fallback at all);
--    an empty model resolves the tier=1 tier_models ladder, each entry
--    carrying its own reasoning_effort (so swapping models can no longer
--    strand a stale per-def effort).
-- 2. tools += read_file,bash — one-shot repo lookups need fs access; an
--    exact CSV entry is operator intent and bypasses the
--    api_native_tools_enabled gate (spawner_api_registry.go csvNamesFSTool).
--    Without fs tools the extractor answered repo questions from priors.
-- 3. api_max_iterations 6 -> 12 — six turns starved honest repo-wide
--    searches into max-iterations deaths on every model evaluated.
-- 4. Prompt gains a search-budget rule so unanswerable questions convert to
--    an early not-found answer; pairs with the runner's near-cap wrap-up
--    notice (runner_loop.go capWarningTurns).

UPDATE system_agent_definitions SET
    model = '',
    tier = 1,
    api_max_iterations = 12,
    tools = 'read_file,bash,findings_add,artifact_get,artifact_list,web_search,web_fetch,read_document,agent_finished,agent_fail,agent_continue,agent_callback,agent_context_update',
    prompt = '## Role: T2 Extractor

You answer exactly one question with exactly one answer. Minimal prose — no preamble, no summary of what you did. Do not explore beyond the specific question you were asked.

## Brief

${DELEGATE_BRIEF}

## Context

${DELEGATE_CONTEXT}

## Item

${DELEGATE_ITEM}

## Artifacts

#{ARTIFACTS}

## Rules

- Budget your search: a few targeted lookups, then answer. A not-found answer with evidence of where you looked beats more searching.
- Record your answer with findings_add, key `_delegate_findings`, value a JSON object `{"answer": "...", "evidence": "..."}`.
- Call agent_finished once findings_add succeeds. If you cannot answer, call agent_fail with the reason.',
    updated_at = datetime('now')
WHERE id = '_t2_extractor';

-- Extend the tier-1 chain with a cross-provider codex hop (rides codex CLI
-- OAuth — no OPENAI_API_KEY needed), only when the chain is still the
-- 000195 two-entry haiku default and the luna model row is usable; a
-- customized chain is left untouched.
INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort)
SELECT 1, 2, 'openai', 'cli_interactive', 'gpt-5.6-luna', 'low'
WHERE (SELECT COUNT(*) FROM tier_models WHERE tier = 1) = 2
  AND (SELECT COUNT(*) FROM tier_models WHERE tier = 1 AND model_id = 'haiku-4-5') = 2
  AND EXISTS (SELECT 1 FROM models WHERE id = 'gpt-5.6-luna' AND enabled = 1);
