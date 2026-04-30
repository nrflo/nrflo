-- Tighten the completion contract wording: drop the "or exit 0" fallback so
-- agents in interactive TUI mode (which cannot exit on their own) reliably call
-- `nrflo agent finished` to signal success.

UPDATE default_templates
SET template = '## Completion Contract

You MUST call exactly one of these CLI commands to finish — do not exit silently.

- **Success**: `nrflo agent finished` — orchestrator advances to the next phase.
- **Failure**: `nrflo agent fail --reason "<reason>"` — orchestrator stops at this layer.
- **Context exhausted**: save progress to the `to_resume` finding, then call `nrflo agent continue` to be relaunched with fresh context.

Run the command via the Bash tool. Interactive sessions do not exit on their own; you must invoke the command explicitly.
',
    default_template = '## Completion Contract

You MUST call exactly one of these CLI commands to finish — do not exit silently.

- **Success**: `nrflo agent finished` — orchestrator advances to the next phase.
- **Failure**: `nrflo agent fail --reason "<reason>"` — orchestrator stops at this layer.
- **Context exhausted**: save progress to the `to_resume` finding, then call `nrflo agent continue` to be relaunched with fresh context.

Run the command via the Bash tool. Interactive sessions do not exit on their own; you must invoke the command explicitly.
',
    updated_at = datetime('now')
WHERE id = 'system-prompt-suffix';

UPDATE default_templates
SET template = '## Before Finishing

Before stopping, confirm you have:
1. Completed the assigned task or clearly identified why it cannot be done.
2. Saved relevant findings with `nrflo findings add`.
3. Run **exactly one** completion command via the Bash tool:
   - `nrflo agent finished` (success)
   - `nrflo agent fail --reason "..."` (failure)

Do not stop without invoking one of these commands.
',
    default_template = '## Before Finishing

Before stopping, confirm you have:
1. Completed the assigned task or clearly identified why it cannot be done.
2. Saved relevant findings with `nrflo findings add`.
3. Run **exactly one** completion command via the Bash tool:
   - `nrflo agent finished` (success)
   - `nrflo agent fail --reason "..."` (failure)

Do not stop without invoking one of these commands.
',
    updated_at = datetime('now')
WHERE id = 'finish-reminder';
