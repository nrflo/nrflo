-- Agents now drive nrflo over MCP tools (mcp__nrflo__* for Claude, nrflo/* for
-- codex) instead of the `nrflo` CLI, which has been removed. Rewrite the two
-- behavioral completion injectables (appended to / nudged into every agent) to
-- reference the nrflo tools instead of CLI commands.

UPDATE default_templates
SET template = '## Completion Contract

You MUST call exactly one of these nrflo tools to finish — do not exit silently.

- **Success**: call the `agent_finished` tool — the orchestrator advances to the next phase.
- **Failure**: call the `agent_fail` tool with a `reason` — the orchestrator stops at this layer.

These tools come from the nrflo MCP server and appear in your tool list (possibly namespaced, e.g. `mcp__nrflo__agent_finished`). Interactive sessions do not exit on their own; you must call the completion tool explicitly.

## Autonomous Run

You are running headless inside an orchestrator. **No human is watching.**

- Never ask clarifying questions, request approval, or pause for confirmation. Make the best decision with the information you have and proceed.
- Do not invite the user to follow up or "let me know if". Just complete the task and call the appropriate completion tool.
- Do not run diagnostic commands to verify your environment. Start the task immediately.
',
    default_template = '## Completion Contract

You MUST call exactly one of these nrflo tools to finish — do not exit silently.

- **Success**: call the `agent_finished` tool — the orchestrator advances to the next phase.
- **Failure**: call the `agent_fail` tool with a `reason` — the orchestrator stops at this layer.

These tools come from the nrflo MCP server and appear in your tool list (possibly namespaced, e.g. `mcp__nrflo__agent_finished`). Interactive sessions do not exit on their own; you must call the completion tool explicitly.

## Autonomous Run

You are running headless inside an orchestrator. **No human is watching.**

- Never ask clarifying questions, request approval, or pause for confirmation. Make the best decision with the information you have and proceed.
- Do not invite the user to follow up or "let me know if". Just complete the task and call the appropriate completion tool.
- Do not run diagnostic commands to verify your environment. Start the task immediately.
',
    updated_at = datetime('now')
WHERE id = 'system-prompt-suffix';

UPDATE default_templates
SET template = '## Before Finishing

Before stopping, confirm you have:
1. Completed the assigned task or clearly identified why it cannot be done.
2. Saved relevant findings with the `findings_add` tool.
3. Called **exactly one** completion tool:
   - `agent_finished` (success)
   - `agent_fail` with a reason (failure)

Do not stop without calling one of these tools.
',
    default_template = '## Before Finishing

Before stopping, confirm you have:
1. Completed the assigned task or clearly identified why it cannot be done.
2. Saved relevant findings with the `findings_add` tool.
3. Called **exactly one** completion tool:
   - `agent_finished` (success)
   - `agent_fail` with a reason (failure)

Do not stop without calling one of these tools.
',
    updated_at = datetime('now')
WHERE id = 'finish-reminder';
