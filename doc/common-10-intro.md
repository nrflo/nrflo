# Agent Authoring — Common Concepts

This section covers concepts that apply across all execution modes
(`cli_interactive`, `script`, `api`). Mode-specific details live in
[cli.md](cli.md), [python.md](python.md), and [api.md](api.md).

Agent definitions configure how agents behave within workflows — their prompts,
models, timeouts, and restart behavior. Definitions are created and edited on
the **Workflows** page: expand a workflow card, then use the **Add Agent**
button or the edit icon on an existing agent.

---

## 1. Template Variables

Template variables are placeholders typed directly into the agent's prompt
template. At runtime, the system substitutes them with actual values.

| Variable | Description | Example Result |
|----------|-------------|----------------|
| `${AGENT}` | Agent type identifier | `implementor` |
| `${TICKET_ID}` | Current ticket ID (empty for project-scope) | `PROJ-42` |
| `${TICKET_TITLE}` | Ticket title (empty for project-scope) | `Fix login bug` |
| `${TICKET_DESCRIPTION}` | Ticket description (empty for project-scope) | full text |
| `${PROJECT_ID}` | Project identifier | `myapp` |
| `${WORKFLOW}` | Workflow name | `feature` |
| `${PARENT_SESSION}` | Parent orchestration session UUID | UUID string |
| `${CHILD_SESSION}` | This agent's session UUID | UUID string |
| `${MODEL_ID}` | Full model identifier | `claude:opus_4_7` |
| `${MODEL}` | Short model name | `opus_4_7` (defaults to `sonnet`) |
| `${NODE_ID}` | Execution node id (the slot in the run; equals `${AGENT}` for static workflows) | `implementor` |

### Ticket Context

For project-scoped workflows, `${TICKET_ID}`, `${TICKET_TITLE}`, and
`${TICKET_DESCRIPTION}` resolve to empty strings. Validation at workflow
creation rejects project-scoped agent prompts that reference these variables.

### Auto-prepended Blocks

These blocks are automatically prepended to the agent prompt when conditions
are met. They are loaded from injectable templates on the Default Templates
page and are user-editable.

| Block | When Prepended | Inner Placeholders |
|-------|---------------|--------------------|
| **User Instructions** | User provided instructions at workflow launch | `${USER_INSTRUCTIONS}` |
| **Low-Context Restart** | Agent saved `to_resume` data before restart | `${PREVIOUS_DATA}` |
| **Callback** | A later-layer agent triggered a callback | `${CALLBACK_INSTRUCTIONS}`, `${CALLBACK_FROM_AGENT}` |

**Prepend order:** user-instructions → low-context → callback.

Legacy `${USER_INSTRUCTIONS}`, `${CALLBACK_INSTRUCTIONS}`, and `${PREVIOUS_DATA}`
placeholders in agent prompts are stripped to empty string at runtime.

### System Prompt Suffix

The `system-prompt-suffix` injectable is delivered separately from the
prepended blocks. For Claude agents it is passed via `--append-system-prompt-file`,
appending it to Claude's system prompt. For codex agents it is
prepended to the prompt body. The suffix template contains the completion
contract (the `agent_finished` / `agent_fail` / `agent_continue` tools, the last
for context exhaustion) and is always active.

The `finish-reminder` injectable is a second readonly template that can be
referenced or appended by workflows to remind agents of the completion contract
just before exiting.
