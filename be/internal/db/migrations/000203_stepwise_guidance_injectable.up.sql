-- Seed a readonly `stepwise-guidance` injectable: the stepwise operating
-- rules + complete_step contract appended to a prompt_mode='stepwise' agent
-- def's rendered prompt (spawner/template_stepwise.go appendStepwiseBlock).
-- template === default_template, matching migration058's repo-wide readonly
-- invariant (verified by migration203_test.go).

INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at) VALUES
    ('stepwise-guidance', 'Stepwise Guidance',
     '## Stepwise Mode

You are working through a server-owned sequence of steps for this task: step ${STEP_INDEX} of ${STEP_TOTAL} — "${STEP_TITLE}" (step_id=${STEP_ID}, revision=${STEP_REVISION}).

- You cannot see step ${STEP_INDEX}''s successor until this step is accepted — do not attempt or pre-answer future steps.
- The server owns the cursor. You advance only by calling `complete_step` with `{step_id, revision, summary, evidence: {finding_keys}}`.
- Record every required finding with `findings_add` BEFORE calling `complete_step` — the call validates against what is already recorded, not what you say you did.
- A rejected `complete_step` call lists exactly what is missing or invalid. Fix and resubmit — never guess at what might satisfy it.
- Use the exact `step_id` and `revision` shown above; a stale revision is rejected.
',
     '## Stepwise Mode

You are working through a server-owned sequence of steps for this task: step ${STEP_INDEX} of ${STEP_TOTAL} — "${STEP_TITLE}" (step_id=${STEP_ID}, revision=${STEP_REVISION}).

- You cannot see step ${STEP_INDEX}''s successor until this step is accepted — do not attempt or pre-answer future steps.
- The server owns the cursor. You advance only by calling `complete_step` with `{step_id, revision, summary, evidence: {finding_keys}}`.
- Record every required finding with `findings_add` BEFORE calling `complete_step` — the call validates against what is already recorded, not what you say you did.
- A rejected `complete_step` call lists exactly what is missing or invalid. Fix and resubmit — never guess at what might satisfy it.
- Use the exact `step_id` and `revision` shown above; a stale revision is rejected.
',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
