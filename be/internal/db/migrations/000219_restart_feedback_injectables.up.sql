-- Seed two readonly injectables for the truthful restart-feedback prepend
-- (spawner/template_restart_feedback.go): `validation-failure` renders when
-- the previous session's relaunch reason is fail_restart and it wrote a
-- genuine validation_failure finding; `timeout-restart` renders for a
-- timeout_restart relaunch. Both replace the `low-context` block for that
-- relaunch (spawner/template.go's prepend seam) rather than stacking with
-- it, so ${PREVIOUS_DATA} is folded in directly.
--
-- The `continuation` injectable seeded by 000054 is dead: no code path
-- renders it any more (this migration's two rows are its replacement for
-- fail_restart/timeout_restart; low-context stays on the unrelated
-- low-context reason).

DELETE FROM default_templates WHERE id = 'continuation';

INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at) VALUES
    ('validation-failure', 'Validation failure restart',
     '## Your Previous Run Failed Validation

Your previous run reported completion, but the following validation command failed:

```
${FAILED_COMMAND}
```

Exit code: ${EXIT_CODE}

Output (tail):
```
${OUTPUT_TAIL}
```

Fix the underlying issue before finishing again — do not just re-run the command.

${PREVIOUS_DATA}',
     '## Your Previous Run Failed Validation

Your previous run reported completion, but the following validation command failed:

```
${FAILED_COMMAND}
```

Exit code: ${EXIT_CODE}

Output (tail):
```
${OUTPUT_TAIL}
```

Fix the underlying issue before finishing again — do not just re-run the command.

${PREVIOUS_DATA}',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('timeout-restart', 'Timeout restart',
     '## Your Previous Run Timed Out

Your previous run was killed for exceeding its time limit. Below is what it saved before being killed:

${PREVIOUS_DATA}',
     '## Your Previous Run Timed Out

Your previous run was killed for exceeding its time limit. Below is what it saved before being killed:

${PREVIOUS_DATA}',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
