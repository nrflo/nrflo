-- Seed two new readonly injectable templates: system-prompt-suffix and finish-reminder.
-- Also clean up residual "just exit 0" guidance in the context-saver system agent prompt.
-- Note: default_templates agent rows (setup-analyzer, test-writer, implementor, qa-verifier,
-- doc-updater, ticket-creator) already have clean content after migration 000058; no UPDATE
-- needed there.

INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at) VALUES
    ('system-prompt-suffix', 'System prompt suffix',
     '## Completion Contract

When your task is complete:
- **Success**: call `nrflo agent continue` (or exit 0)
- **Failure**: call `nrflo agent fail --reason "<reason>"`

Always explicitly report your result — do not exit silently.
',
     '## Completion Contract

When your task is complete:
- **Success**: call `nrflo agent continue` (or exit 0)
- **Failure**: call `nrflo agent fail --reason "<reason>"`

Always explicitly report your result — do not exit silently.
',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('finish-reminder', 'Finish reminder',
     '## Before Finishing

Before exiting, confirm you have:
1. Completed the assigned task or clearly identified why it cannot be done
2. Saved relevant findings with `nrflo findings add`
3. Called `nrflo agent continue` (success) or `nrflo agent fail --reason "..."` (failure)
',
     '## Before Finishing

Before exiting, confirm you have:
1. Completed the assigned task or clearly identified why it cannot be done
2. Saved relevant findings with `nrflo findings add`
3. Called `nrflo agent continue` (success) or `nrflo agent fail --reason "..."` (failure)
',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

-- Remove the "Do NOT call / just exit 0" guidance from the context-saver CLI agent.
-- The agent saves findings and exits 0 — calling agent continue is equivalent and preferred.
UPDATE system_agent_definitions
SET prompt = REPLACE(
        prompt,
        '- Do NOT call `nrflo agent continue` or `nrflo agent fail` — just exit 0 after saving findings',
        '- After saving findings, exit 0 or call `nrflo agent continue`'
    ),
    updated_at = datetime('now')
WHERE id = 'context-saver';
