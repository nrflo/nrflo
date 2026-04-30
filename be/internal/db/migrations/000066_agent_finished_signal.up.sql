-- Introduce explicit `nrflo agent finished` signal for success completion.
-- Update readonly injectable templates so agents call `agent finished` when done
-- instead of `agent continue` (continue is reserved for context-exhaustion relaunch).

UPDATE default_templates
SET template = '## Completion Contract

When your task is complete:
- **Success**: call `nrflo agent finished` (or exit 0 for an implicit pass)
- **Failure**: call `nrflo agent fail --reason "<reason>"`
- **Context exhausted**: save progress to the `to_resume` finding, then call `nrflo agent continue` to be relaunched with fresh context.

Always explicitly report your result — do not exit silently.
',
    default_template = '## Completion Contract

When your task is complete:
- **Success**: call `nrflo agent finished` (or exit 0 for an implicit pass)
- **Failure**: call `nrflo agent fail --reason "<reason>"`
- **Context exhausted**: save progress to the `to_resume` finding, then call `nrflo agent continue` to be relaunched with fresh context.

Always explicitly report your result — do not exit silently.
',
    updated_at = datetime('now')
WHERE id = 'system-prompt-suffix';

UPDATE default_templates
SET template = '## Before Finishing

Before exiting, confirm you have:
1. Completed the assigned task or clearly identified why it cannot be done
2. Saved relevant findings with `nrflo findings add`
3. Called `nrflo agent finished` (success) or `nrflo agent fail --reason "..."` (failure)
',
    default_template = '## Before Finishing

Before exiting, confirm you have:
1. Completed the assigned task or clearly identified why it cannot be done
2. Saved relevant findings with `nrflo findings add`
3. Called `nrflo agent finished` (success) or `nrflo agent fail --reason "..."` (failure)
',
    updated_at = datetime('now')
WHERE id = 'finish-reminder';
