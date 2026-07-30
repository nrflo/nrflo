-- Rewrite the readonly `delegation-guidance` injectable: the 000188 text
-- instructed "poll `get_delegation` for the result", which reliably produced
-- a bare get_delegation call per model turn (13 junk turns observed on a
-- 30s delegation — each a full caller-model turn, replayed as context
-- forever after). The tool now blocks inline by default for extractor and
-- hints wait_sec on async returns; the guidance matches. Also drops the
-- stale pre-tier "T2" naming. Readonly row: template == default_template.

UPDATE default_templates SET template = '## Delegation

- Delegate broad exploration to cheap-tier workers via the `dynamic` sub-workflow rather than doing it inline.
- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — it blocks inline by default and returns the result in one call.
- Ask delegates for structured findings and evidence, never raw transcripts or tool output.
- Go async (`wait_sec: 0`) only for fanouts or long executor jobs; collect with ONE `get_delegation` call passing `wait_sec` — never call `get_delegation` repeatedly in a loop.
- Respect the delegation depth limit: do not re-delegate past the cap.
', updated_at = CURRENT_TIMESTAMP WHERE id = 'delegation-guidance' AND readonly = 1;
UPDATE default_templates SET default_template = template, updated_at = CURRENT_TIMESTAMP WHERE id = 'delegation-guidance' AND readonly = 1;
