-- Revert system-prompt-suffix to the 000067 wording (no Autonomous Run section).

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
