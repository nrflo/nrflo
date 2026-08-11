-- Third delegate tier: verifier. Session audits showed extractor workers
-- (tier-1 ladder, haiku-class) routinely producing wrong data on exactly one
-- class of brief — adversarial verification (absence claims, contradictions
-- between workers, audit-critical positives) — forcing deciders to burn 2-3x
-- re-check fanouts, with one fabricated claim reaching a committed doc.
-- Mechanical lookups were fine on the cheap tier. Split the role instead of
-- bumping every extractor: `verifier` is the same one-shot no-recursion
-- contract as extractor but resolves the tier-2 ladder (sonnet-class low)
-- and carries a refute-by-default prompt.

INSERT INTO system_agent_definitions (
    id, role, model, timeout, prompt, tools, api_max_iterations,
    stall_start_timeout_sec, stall_running_timeout_sec, execution_mode,
    reasoning_effort, tier, isolate_worktree, created_at, updated_at
) VALUES (
    '_t3_verifier',
    'verifier',
    '',
    10,
    '## Role: T3 Verifier

You adversarially verify exactly one claim. Assume the claim is wrong until real evidence forces you to concede — do not confirm out of politeness, and never reason from naming, plausibility, or memory.

## Brief

${DELEGATE_BRIEF}

## Context

${DELEGATE_CONTEXT}

## Item

${DELEGATE_ITEM}

## Artifacts

#{ARTIFACTS}

## Rules

- Open the files and run the commands yourself; quote file:line or the exact command with its literal output. Never state that something exists (or is absent) without pasted evidence.
- A zero count or not-found is a valid answer — report it as such, do not soften or invert it.
- Follow chains to the end: a citation that does not match, a guard that dead-ends the path, or missing data is itself a finding.
- Record your answer with findings_add, key `_delegate_findings`, value a JSON object `{"verdict": "confirmed|refuted|partial|undetermined", "answer": "...", "evidence": "..."}`.
- Call agent_finished once findings_add succeeds. If you cannot verify, call agent_fail with the reason.',
    'read_file,bash,findings_add,artifact_get,artifact_list,web_search,web_fetch,read_document,agent_finished,agent_fail,agent_continue,agent_callback,agent_context_update',
    12,
    60,
    180,
    'api',
    'low',
    2,
    0,
    datetime('now'),
    datetime('now')
);

-- Executor prompt: point adversarial re-checks at the new tier.
UPDATE system_agent_definitions
SET prompt = REPLACE(prompt,
    'delegate to a T2 extractor (the `delegate` tool, tier="extractor") rather than open-ended exploration.',
    'delegate to a T2 extractor (the `delegate` tool, tier="extractor") rather than open-ended exploration; for adversarial re-checks of a specific claim (absence claims, contradictions between workers, audit-critical positives) use tier="verifier" instead.'),
    updated_at = datetime('now')
WHERE id = '_t1_executor';

-- Delegation-guidance injectable: add the verifier routing bullet after the
-- extractor bullet. Readonly row: template == default_template.
UPDATE default_templates
SET template = REPLACE(template,
    '- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — it blocks inline by default and returns the result in one call.',
    '- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — it blocks inline by default and returns the result in one call.
- Route adversarial re-checks — absence claims, contradictions between workers, audit-critical positives — to a `verifier` tier worker: same one-shot inline contract as extractor, stronger model, refute-by-default.'),
    default_template = REPLACE(default_template,
    '- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — it blocks inline by default and returns the result in one call.',
    '- Delegate one-shot lookups to an `extractor` tier worker via `delegate` — it blocks inline by default and returns the result in one call.
- Route adversarial re-checks — absence claims, contradictions between workers, audit-critical positives — to a `verifier` tier worker: same one-shot inline contract as extractor, stronger model, refute-by-default.'),
    updated_at = CURRENT_TIMESTAMP
WHERE id = 'delegation-guidance';

-- t0 templates: 000235 told deciders to re-check claims with a fresh
-- extractor per claim — the exact pattern the audits showed failing (a
-- haiku re-check of a haiku claim split 2-1 at least once). Point the
-- verification pass at the verifier tier, and fold verifier into the
-- 000234 inline-return contract.
UPDATE default_templates
SET template = REPLACE(REPLACE(template,
    '(a fresh extractor per claim, briefed to refute it)',
    '(a fresh `verifier`-tier worker per claim, briefed to refute it)'),
    'Extractor delegations return their findings inline',
    'Extractor and verifier delegations return their findings inline'),
    default_template = REPLACE(REPLACE(default_template,
    '(a fresh extractor per claim, briefed to refute it)',
    '(a fresh `verifier`-tier worker per claim, briefed to refute it)'),
    'Extractor delegations return their findings inline',
    'Extractor and verifier delegations return their findings inline'),
    updated_at = CURRENT_TIMESTAMP
WHERE id IN ('tier-t0-decider', 'tier-t0-bare');
