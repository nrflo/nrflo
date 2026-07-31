-- Restore the 000224 delegation-guidance text (manual review/merge hint).

UPDATE default_templates SET template = '## Delegation

- Delegate broad exploration to cheap-tier workers via the `dynamic` sub-workflow rather than doing it inline.
- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — it blocks inline by default and returns the result in one call.
- Ask delegates for structured findings and evidence, never raw transcripts or tool output.
- Go async (`wait_sec: 0`) only for fanouts or long executor jobs; collect with ONE `get_delegation` call passing `wait_sec` — never call `get_delegation` repeatedly in a loop.
- Respect the delegation depth limit: do not re-delegate past the cap.
- Executor-tier results land on a server-committed branch, not the live tree — `get_delegation` reports the branch so you or the user can review/merge it.
', updated_at = CURRENT_TIMESTAMP WHERE id = 'delegation-guidance' AND readonly = 1;
UPDATE default_templates SET default_template = template, updated_at = CURRENT_TIMESTAMP WHERE id = 'delegation-guidance' AND readonly = 1;
