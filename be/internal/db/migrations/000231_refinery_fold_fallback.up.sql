-- Refinery fold fallback: the fold core now walks the `_refinery` tier-1
-- chain (mirroring spawnEntryWithBuildFallback) instead of only ever
-- consuming chain[0]. refinery_runs gains three columns recording which
-- chain entry each attempt landed on, mirroring agent_sessions'
-- chain_position/fallback_from/resolved_execution_mode (000198).
ALTER TABLE refinery_runs ADD COLUMN chain_position INTEGER NOT NULL DEFAULT 0;
ALTER TABLE refinery_runs ADD COLUMN fallback_from TEXT NOT NULL DEFAULT '';
ALTER TABLE refinery_runs ADD COLUMN execution_mode TEXT NOT NULL DEFAULT '';

-- Seed the cli_interactive sibling of `_refinery` (role='refinery'), the
-- same pairing shape as context-saver/context-saver-api (000063): the api
-- row stays the tier's primary entry, this row is what a cli_interactive
-- tier_models entry (already seeded at tier 1 position >= 1, see 000195/
-- 000220) spawns as a one-off headless child via
-- spawner.Spawner.RunRefineryFold. model='' so it resolves from whichever
-- chain entry landed on it rather than a per-def pin.
INSERT INTO system_agent_definitions (
    id, role, model, timeout, prompt, tools,
    stall_start_timeout_sec, stall_running_timeout_sec, execution_mode, tier,
    created_at, updated_at
) VALUES (
    '_refinery-cli',
    'refinery',
    '',
    3,
    '# Working-Set Refinery (CLI)

You maintain a compact working-set digest for an ongoing conversation between a user (or an autonomous session) and an AI agent. The digest''s subject is the conversation itself: what the user asked for, what the agent decided and did, and any delegation findings it consumed — never the surrounding event plumbing.

## Input

${FOLD_INPUT}

The input above carries, in order: an optional `## Task` section (the session''s immutable assigned task, supplied verbatim every fold — anchor the digest to it, never summarize/drop/contradict it, and do not restate it in your output), the `## Previous Digest`, a `## Conversation` section (categorized message-delta lines), and an optional `## New Events` section (finding-update/workflow/plan-state metadata lines, secondary context only — never let it become the digest''s subject).

Write narrative only: capture intent, reasoning, decisions, blockers and open questions. Do NOT enumerate file paths, line numbers, ticket IDs, or command strings. Refer to code by role or responsibility, not by exact path.

Cover, in order: goal, constraints, plan state, active findings, open questions. Drop stale or superseded detail rather than growing unbounded — the digest must never exceed 4000 bytes.

## Task

Call the `findings_add` tool with:
- key: `_refinery_digest`
- value: the updated digest text — no preamble, no code fences, no commentary

Then call `agent_finished`. Call `findings_add` exactly once with key=_refinery_digest.',
    'findings_add,agent_finished',
    60,
    120,
    'cli_interactive',
    1,
    datetime('now'),
    datetime('now')
);
