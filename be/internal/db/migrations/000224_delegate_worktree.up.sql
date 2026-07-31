-- Per-delegation worktree isolation for write-capable executor tiers.
--
-- delegations gains worktree metadata (path/branch/base commit/summary),
-- written by the server once the fanout's workers finish and their combined
-- diff has been committed; NULL/'' when the delegation ran in-place (no
-- isolation configured, run-less-only, or git unavailable).
--
-- system_agent_definitions gains isolate_worktree: a data-driven flag (not a
-- tier-name check at the call site) that opts a tier definition into
-- per-delegation worktree isolation. Only _t1_executor is isolated by
-- default — _t2_extractor stays read-mostly and runs in the live tree.
ALTER TABLE delegations ADD COLUMN worktree_path TEXT NOT NULL DEFAULT '';
ALTER TABLE delegations ADD COLUMN branch_name TEXT NOT NULL DEFAULT '';
ALTER TABLE delegations ADD COLUMN base_commit TEXT NOT NULL DEFAULT '';
ALTER TABLE delegations ADD COLUMN worktree_summary TEXT NOT NULL DEFAULT '';

ALTER TABLE system_agent_definitions ADD COLUMN isolate_worktree INTEGER NOT NULL DEFAULT 0;

UPDATE system_agent_definitions SET isolate_worktree = 1 WHERE id = '_t1_executor';

-- The executor is told it is running on a disposable, server-owned branch:
-- committing/pushing/switching branches itself would race the server's own
-- commit-on-completion step.
UPDATE system_agent_definitions SET
    prompt = '## Role: T1 Executor

You own the slice of work you were given end to end. For one-shot lookups within your slice, delegate to a T2 extractor (the `delegate` tool, tier="extractor") rather than open-ended exploration. Report structured findings, never raw transcripts.

## Brief

${DELEGATE_BRIEF}

## Context

${DELEGATE_CONTEXT}

## Item

${DELEGATE_ITEM}

## Artifacts

#{ARTIFACTS}

## Rules

- Large payloads (diffs, logs, command output) go to artifacts, not inline into your report.
- Record your findings with findings_add, key `_delegate_findings`, value a JSON object summarizing what you did and what you found.
- Call agent_finished once findings_add succeeds. If you cannot complete the work, call agent_fail with the reason.
- You are on a disposable branch — never commit, push, or switch branches yourself; the server commits your working tree once you finish.',
    updated_at = datetime('now')
WHERE id = '_t1_executor';

UPDATE default_templates SET template = '## Delegation

- Delegate broad exploration to cheap-tier workers via the `dynamic` sub-workflow rather than doing it inline.
- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — it blocks inline by default and returns the result in one call.
- Ask delegates for structured findings and evidence, never raw transcripts or tool output.
- Go async (`wait_sec: 0`) only for fanouts or long executor jobs; collect with ONE `get_delegation` call passing `wait_sec` — never call `get_delegation` repeatedly in a loop.
- Respect the delegation depth limit: do not re-delegate past the cap.
- Executor-tier results land on a server-committed branch, not the live tree — `get_delegation` reports the branch so you or the user can review/merge it.
', updated_at = CURRENT_TIMESTAMP WHERE id = 'delegation-guidance' AND readonly = 1;
UPDATE default_templates SET default_template = template, updated_at = CURRENT_TIMESTAMP WHERE id = 'delegation-guidance' AND readonly = 1;
