-- Seed a readonly `delegation-guidance` injectable: tight delegation rules
-- appended to the rendered system prompt (spawner/template_injectable.go
-- appendDelegationGuidance) ONLY when a def's effective tools include
-- `delegate`. Also trims the delegation how-to now owned by this injectable
-- out of tier-t0-decider/tier-t1-executor (seeded by 000178), keeping their
-- role framing, so tier + guidance compose without duplicated instructions.

INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at) VALUES
    ('delegation-guidance', 'Delegation Guidance',
     '## Delegation

- Delegate broad exploration to cheap-tier workers via the `dynamic` sub-workflow rather than doing it inline.
- Delegate one-shot lookups to a T2 `extractor` via `delegate`.
- Ask delegates for structured findings and evidence, never raw transcripts or tool output.
- `delegate` returns async by default — poll `get_delegation` for the result.
- Respect the delegation depth limit: do not re-delegate past the cap.
',
     '## Delegation

- Delegate broad exploration to cheap-tier workers via the `dynamic` sub-workflow rather than doing it inline.
- Delegate one-shot lookups to a T2 `extractor` via `delegate`.
- Ask delegates for structured findings and evidence, never raw transcripts or tool output.
- `delegate` returns async by default — poll `get_delegation` for the result.
- Respect the delegation depth limit: do not re-delegate past the cap.
',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

-- --- tier-t0-decider: trim delegation how-to, keep role framing ---
UPDATE default_templates SET default_template = '## Role: T0 Decider

You decide, plan, judge, and synthesize only. You never grep, read, edit, or run commands yourself — every execution step is delegated.

- Hard rule: if you feel the urge to grep, read a file, or edit something yourself, that is the signal to delegate it instead.
', updated_at = CURRENT_TIMESTAMP WHERE id = 'tier-t0-decider' AND readonly = 1;
UPDATE default_templates SET template = default_template, updated_at = CURRENT_TIMESTAMP WHERE id = 'tier-t0-decider' AND readonly = 1;

-- --- tier-t1-executor: trim delegation how-to, keep role framing ---
UPDATE default_templates SET default_template = '## Role: T1 Executor

You own the slice of work you were given end to end.

- Large payloads (diffs, logs, command output) go to artifacts, not inline into your report.
', updated_at = CURRENT_TIMESTAMP WHERE id = 'tier-t1-executor' AND readonly = 1;
UPDATE default_templates SET template = default_template, updated_at = CURRENT_TIMESTAMP WHERE id = 'tier-t1-executor' AND readonly = 1;
