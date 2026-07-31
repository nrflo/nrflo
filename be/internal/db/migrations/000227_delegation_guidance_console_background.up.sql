-- Under a Claude Code console engine, a `get_delegation`/`delegate` bounded
-- wait over ~120s is backgrounded by the harness and its result surfaces
-- later as a task notification carrying a non-terminal status — that
-- notification does not consume the delegation. Document the extra
-- `get_delegation` call the caller must make, so console agents don't treat
-- the backgrounding as an error or a signal to stop.
-- Readonly row: template == default_template.

UPDATE default_templates SET template = '## Delegation

- Delegate broad exploration to cheap-tier workers via the `dynamic` sub-workflow rather than doing it inline.
- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — it blocks inline by default and returns the result in one call.
- Ask delegates for structured findings and evidence, never raw transcripts or tool output.
- Go async (`wait_sec: 0`) only for fanouts or long executor jobs; collect with ONE `get_delegation` call passing `wait_sec` — never call `get_delegation` repeatedly in a loop.
- Under a CLI console engine a wait over ~120s may return as a background-task notification carrying a non-terminal status — that notification does not consume the delegation: call `get_delegation` once more after it, and never treat the backgrounding as an error.
- Respect the delegation depth limit: do not re-delegate past the cap.
- Executor-tier results land on a server-committed branch, not the live tree — review the findings, then land the branch with `merge_delegation`. Never merge it by hand (bash/git or a delegated worker): the server merge refuses a dirty tree, aborts cleanly on conflict, and keeps the delegation record honest.
', updated_at = CURRENT_TIMESTAMP WHERE id = 'delegation-guidance' AND readonly = 1;
UPDATE default_templates SET default_template = template, updated_at = CURRENT_TIMESTAMP WHERE id = 'delegation-guidance' AND readonly = 1;
