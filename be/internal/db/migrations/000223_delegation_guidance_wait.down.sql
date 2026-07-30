UPDATE default_templates SET template = '## Delegation

- Delegate broad exploration to cheap-tier workers via the `dynamic` sub-workflow rather than doing it inline.
- Delegate one-shot lookups to a T2 `extractor` via `delegate`.
- Ask delegates for structured findings and evidence, never raw transcripts or tool output.
- `delegate` returns async by default — poll `get_delegation` for the result.
- Respect the delegation depth limit: do not re-delegate past the cap.
', updated_at = CURRENT_TIMESTAMP WHERE id = 'delegation-guidance' AND readonly = 1;
UPDATE default_templates SET default_template = template, updated_at = CURRENT_TIMESTAMP WHERE id = 'delegation-guidance' AND readonly = 1;
