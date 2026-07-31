UPDATE default_templates SET template = '## Delegation

- Delegate broad exploration to cheap-tier workers via the `dynamic` sub-workflow rather than doing it inline.
- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — it blocks inline by default and returns the result in one call.
- Ask delegates for structured findings and evidence, never raw transcripts or tool output.
- Go async (`wait_sec: 0`) only for fanouts or long executor jobs; collect with ONE `get_delegation` call passing `wait_sec` — never call `get_delegation` repeatedly in a loop.
- Respect the delegation depth limit: do not re-delegate past the cap.
', updated_at = CURRENT_TIMESTAMP WHERE id = 'delegation-guidance' AND readonly = 1;
UPDATE default_templates SET default_template = template, updated_at = CURRENT_TIMESTAMP WHERE id = 'delegation-guidance' AND readonly = 1;

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
- Call agent_finished once findings_add succeeds. If you cannot complete the work, call agent_fail with the reason.',
    updated_at = datetime('now')
WHERE id = '_t1_executor';

ALTER TABLE system_agent_definitions DROP COLUMN isolate_worktree;

ALTER TABLE delegations DROP COLUMN worktree_summary;
ALTER TABLE delegations DROP COLUMN base_commit;
ALTER TABLE delegations DROP COLUMN branch_name;
ALTER TABLE delegations DROP COLUMN worktree_path;
