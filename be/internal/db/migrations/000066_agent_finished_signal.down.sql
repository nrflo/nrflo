-- Revert injectable templates to the agent-continue-as-success wording.

UPDATE default_templates
SET template = '## Completion Contract

When your task is complete:
- **Success**: call `nrflo agent continue` (or exit 0)
- **Failure**: call `nrflo agent fail --reason "<reason>"`

Always explicitly report your result — do not exit silently.
',
    default_template = '## Completion Contract

When your task is complete:
- **Success**: call `nrflo agent continue` (or exit 0)
- **Failure**: call `nrflo agent fail --reason "<reason>"`

Always explicitly report your result — do not exit silently.
',
    updated_at = datetime('now')
WHERE id = 'system-prompt-suffix';

UPDATE default_templates
SET template = '## Before Finishing

Before exiting, confirm you have:
1. Completed the assigned task or clearly identified why it cannot be done
2. Saved relevant findings with `nrflo findings add`
3. Called `nrflo agent continue` (success) or `nrflo agent fail --reason "..."` (failure)
',
    default_template = '## Before Finishing

Before exiting, confirm you have:
1. Completed the assigned task or clearly identified why it cannot be done
2. Saved relevant findings with `nrflo findings add`
3. Called `nrflo agent continue` (success) or `nrflo agent fail --reason "..."` (failure)
',
    updated_at = datetime('now')
WHERE id = 'finish-reminder';
