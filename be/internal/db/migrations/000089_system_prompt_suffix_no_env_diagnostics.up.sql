-- Append a "no env diagnostics" rule to the system-prompt-suffix injectable.
-- Agents sometimes waste turns verifying their environment before starting work;
-- this bullet makes it explicit that the nrflo CLI is already on PATH.

UPDATE default_templates
SET template = '## Completion Contract

You MUST call exactly one of these CLI commands to finish — do not exit silently.

- **Success**: `nrflo agent finished` — orchestrator advances to the next phase.
- **Failure**: `nrflo agent fail --reason "<reason>"` — orchestrator stops at this layer.

Run the command via the Bash tool. Interactive sessions do not exit on their own; you must invoke the command explicitly.

## Autonomous Run

You are running headless inside an orchestrator. **No human is watching.**

- Never ask clarifying questions, request approval, or pause for confirmation. Make the best decision with the information you have and proceed.
- Do not invite the user to follow up or "let me know if". Just complete the task and call the appropriate completion command.
- Do not run diagnostic commands to verify your environment — the nrflo CLI is on PATH and configured. Start the task immediately.
',
    default_template = '## Completion Contract

You MUST call exactly one of these CLI commands to finish — do not exit silently.

- **Success**: `nrflo agent finished` — orchestrator advances to the next phase.
- **Failure**: `nrflo agent fail --reason "<reason>"` — orchestrator stops at this layer.

Run the command via the Bash tool. Interactive sessions do not exit on their own; you must invoke the command explicitly.

## Autonomous Run

You are running headless inside an orchestrator. **No human is watching.**

- Never ask clarifying questions, request approval, or pause for confirmation. Make the best decision with the information you have and proceed.
- Do not invite the user to follow up or "let me know if". Just complete the task and call the appropriate completion command.
- Do not run diagnostic commands to verify your environment — the nrflo CLI is on PATH and configured. Start the task immediately.
',
    updated_at = datetime('now')
WHERE id = 'system-prompt-suffix';
