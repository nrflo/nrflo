# Agent Definition Manual

> Last updated: 2026-03-31

Guide for creating and managing agent definitions in the nrflo web UI. Agent definitions configure how AI agents behave within workflows — their prompts, models, timeouts, and restart behavior.

Agent definitions are created and edited on the **Workflows** page: expand a workflow card, then use the **Add Agent** button or the edit icon on an existing agent.

---

## 1. Template Variables

Template variables are placeholders you type directly into the agent's prompt template (the CodeMirror editor in the agent form). At runtime, the system substitutes them with actual values.

| Variable | Description | How to Use | Result |
|----------|-------------|------------|--------|
| `${AGENT}` | The agent's type identifier | `You are the ${AGENT} agent.` | `You are the implementor agent.` |
| `${TICKET_ID}` | Current ticket ID (empty for project-scope) | `Working on ticket ${TICKET_ID}` | `Working on ticket PROJ-42` |
| `${TICKET_TITLE}` | Ticket title (empty for project-scope) | `## Ticket: ${TICKET_TITLE}` | `## Ticket: Fix login bug` |
| `${TICKET_DESCRIPTION}` | Ticket description (empty for project-scope) | `${TICKET_DESCRIPTION}` | The full ticket description text |
| `${PROJECT_ID}` | Project identifier | `Project: ${PROJECT_ID}` | `Project: myapp` |
| `${WORKFLOW}` | Workflow name | `Running in ${WORKFLOW} workflow` | `Running in feature workflow` |
| `${PARENT_SESSION}` | Parent orchestration session UUID | `${PARENT_SESSION}` | UUID string |
| `${CHILD_SESSION}` | This agent's session UUID | `${CHILD_SESSION}` | UUID string |
| `${MODEL_ID}` | Full model identifier | `${MODEL_ID}` | `claude:opus_4_7` |
| `${MODEL}` | Short model name | `${MODEL}` | `opus_4_7` (defaults to `sonnet`) |

### Ticket Context

For project-scoped workflows, `${TICKET_ID}`, `${TICKET_TITLE}`, and `${TICKET_DESCRIPTION}` resolve to empty strings. Validation at workflow creation rejects project-scoped agent prompts that use these variables.

### Auto-prepended Blocks

The following blocks are automatically prepended to the agent prompt when conditions are met. They are loaded from injectable templates in the Default Templates page and are user-editable.

| Block | When Prepended | Inner Placeholders |
|-------|---------------|-------------------|
| **User Instructions** | User provided instructions at workflow launch | `${USER_INSTRUCTIONS}` |
| **Low-Context Restart** | Agent saved `to_resume` data before restart | `${PREVIOUS_DATA}` |
| **Callback** | A later-layer agent triggered a callback | `${CALLBACK_INSTRUCTIONS}`, `${CALLBACK_FROM_AGENT}` |

**Prepend order:** user-instructions → low-context → callback.

Legacy `${USER_INSTRUCTIONS}`, `${CALLBACK_INSTRUCTIONS}`, and `${PREVIOUS_DATA}` placeholders in agent prompts are stripped to empty.

---

## 2. Findings Patterns

Findings patterns pull data from other agents or project-level findings into an agent's prompt. They are expanded after variable substitution.

### Agent Findings (`#{FINDINGS:...}`)

Pull prior agent findings into prompts.

**Syntax:**

```markdown
#{FINDINGS:setup-analyzer}
#{FINDINGS:setup-analyzer:summary}
#{FINDINGS:setup-analyzer:summary,files_to_modify}
```

**Output format (single agent):**

```
summary: Analysis found 3 files to modify
files_to_modify:
  - src/handler.go
  - src/service.go
```

**Output format (parallel agents — multiple models):**

```
- setup-analyzer:claude:opus:
  summary: Analysis found 3 files to modify
- setup-analyzer:claude:sonnet:
  summary: Found pattern in auth module
```

When requesting a single key from parallel agents:

```
- setup-analyzer:claude:opus: Analysis found 3 files
- setup-analyzer:claude:sonnet: Found pattern in auth module
```

**Missing findings placeholder:**

```
_No findings yet available from setup-analyzer_
```

### Project Findings (`#{PROJECT_FINDINGS:...}`)

Pull project-level findings into prompts.

**Syntax:**

```markdown
#{PROJECT_FINDINGS:architecture}
#{PROJECT_FINDINGS:architecture,conventions}
```

**Single key output:** Returns the value directly (no key prefix).

**Multiple keys output:**

```
architecture: Monorepo with Go backend and React frontend
conventions: Use camelCase for JS, snake_case for Go
```

**Missing key placeholder:**

```
_No project finding for key 'architecture'_
```

For multiple keys, each missing key gets its own placeholder while found keys display normally.

---

## 3. Agent Lifecycle Commands

Spawned agents report results via CLI. **Exiting with code 0 is an implicit pass** — no explicit completion call needed. Only call `agent fail` when the task cannot be completed. Context is provided automatically by the system.

```bash
# Mark agent as failed
nrflo agent fail [--reason <text>]

# Signal context exhaustion — triggers relaunch with fresh context
nrflo agent continue

# Callback to re-run an earlier layer
nrflo agent callback --level <N>

# Skip a workflow group tag
nrflo skip <tag>
```

| Command | When to use |
|---------|------------|
| `fail` | Task cannot be completed; `--reason` is optional but recommended |
| `continue` | Context window exhausted; save progress to `to_resume` first |
| `callback` | Issue found that requires re-running an earlier layer; `--level` (0-based layer index) is required |
| `skip <tag>` | Skip a workflow group in subsequent layers; tag must be in workflow's `groups` |

**Completion semantics:** Exit 0 = pass (immediate, no grace period). Exit non-zero = fail. Only use `agent fail` for explicit failure with a reason.

---

## 4. Findings CLI Commands

### Agent-Level Findings

```bash
# Add single finding (own session)
nrflo findings add <key> <value>

# Add multiple findings (batch syntax)
nrflo findings add key1:val1 key2:val2

# Append to existing finding (creates array if needed)
nrflo findings append <key> <value>
nrflo findings append key1:val1 key2:val2

# Get own findings
nrflo findings get                      # all own findings
nrflo findings get <key>               # single key (positional)
nrflo findings get -k key1 -k key2    # multiple keys

# Get another agent's findings (cross-agent read)
nrflo findings get <agent-type>             # all findings for agent
nrflo findings get <agent-type> <key>      # single key
nrflo findings get <agent-type> -k key1    # multiple keys

# Delete findings
nrflo findings delete <key1> [key2...]
```

### Project-Level Findings

```bash
# Add
nrflo findings project-add <key> <value>
nrflo findings project-add key1:val1 key2:val2

# Get
nrflo findings project-get                    # all project findings
nrflo findings project-get <key>              # single key
nrflo findings project-get -k key1 -k key2    # multiple keys

# Append
nrflo findings project-append <key> <value>
nrflo findings project-append key1:val1 key2:val2

# Delete
nrflo findings project-delete <key1> [key2...]
```

### Batch Syntax

Both `add` and `append` support `key:value` pairs. The first colon separates the key from the value:

```bash
nrflo findings add summary:'Fixed the auth bug' files_changed:'["auth.go"]'
```

---

## 5. Workflow Result

Any agent can write a `workflow_final_result` finding to surface a human-readable result summary after workflow completion.

### How to Set It

```bash
nrflo findings add workflow_final_result:"Implementation complete: added auth middleware with JWT validation"
```

### Behavior

- The value appears as a top-level field in the workflow state API response (`workflow_final_result`)
- Displayed in the UI above the agent flow tree after workflow completion
- **Last-writer-wins:** if multiple agents write `workflow_final_result`, the value from the session with the latest `ended_at` is used
- If no agent writes this finding, no result is displayed

---

## 6. Agent Definition Fields

These fields are configured via the agent form on the **Workflows** page.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `id` | string | Required | Agent type identifier (e.g., `setup-analyzer`, `implementor`) |
| `layer` | int | `0` | Phase execution layer (integer >= 0). Agents with the same layer run concurrently; layers execute in ascending order. |
| `model` | string | `sonnet` | Model to use (see table below) |
| `timeout` | int | `20` | Max execution time in minutes |
| `prompt` | string | Required | Prompt template with `${VAR}` and `#{FINDINGS:...}` patterns. Edited in the CodeMirror markdown editor. |
| `restart_threshold` | int | `25` | Percentage of context remaining that triggers low-context save (lower = more aggressive) |
| `max_fail_restarts` | int | `0` | Max auto-restarts on agent failure (0 = disabled). Failed agent is relaunched fresh (no context save). |
| `stall_start_timeout_sec` | int | Configurable | Seconds with no output before start-stall restart. `0` = disabled. |
| `stall_running_timeout_sec` | int | Configurable | Seconds of silence mid-execution before running-stall restart. `0` = disabled. |
| `tag` | string | `""` | Group tag for skip-tag feature; must be in parent workflow's `groups` |
| `low_consumption_model` | string | `""` | Model override when low consumption mode is enabled globally (e.g., `sonnet`, `haiku`). Empty = no override. |

### Supported Models by CLI Adapter

**Claude (`claude` CLI):**

| Model value | Maps to |
|-------------|---------|
| `opus_4_6` | Claude Opus 4.6 (200k context) |
| `opus_4_6_1m` | Claude Opus 4.6 (1M context) |
| `opus_4_7` | Claude Opus 4.7 (200k context) |
| `opus_4_7_1m` | Claude Opus 4.7 (1M context) |
| `sonnet` | Claude Sonnet |
| `haiku` | Claude Haiku |

**Opencode (`opencode` CLI):**

| Model value | Maps to |
|-------------|---------|
| `opencode_minimax_m25_free` | `opencode/minimax-m2.5-free` |
| `opencode_qwen36_plus_free` | `opencode/qwen3.6-plus-free` |
| `opencode_gpt54` | `openai/gpt-5.4` with `--variant high` |

**Codex (`codex` CLI):**

| Model value | Maps to |
|-------------|---------|
| `codex_gpt_normal` | `gpt-5.3-codex` reasoning effort "high" |
| `codex_gpt_high` | `gpt-5.3-codex` reasoning effort "high" |

---

## 7. Workflow & Phase Configuration

### Phase Configuration

Phases are defined by agent definitions. Each agent definition has an `id` and a `layer` field (integer >= 0). The workflow's phases are derived from its agent definitions at read time, ordered by `layer ASC, id ASC`. For example, a workflow with these agent definitions:

| Agent ID | Layer |
|----------|-------|
| setup-analyzer | 0 |
| test-writer | 1 |
| implementor | 2 |
| qa-verifier | 3 |

produces the phase order: setup-analyzer -> test-writer -> implementor -> qa-verifier.

### Layer Execution Rules

- **Concurrent execution:** All agents in the same layer run concurrently
- **Sequential layers:** Layers execute in ascending order (0, 1, 2, ...)
- **Fan-in validation:** If layer N has >1 agent, layer N+1 must have exactly 1 agent
- **Pass condition:** At least 1 agent in a layer must pass for the workflow to proceed
- **All skipped:** If all agents in a layer are skipped, the workflow continues

### Workflow Groups (Skip Tags)

Workflows can define `groups` — an array of strings (e.g., `["be", "fe", "docs"]`). Agents can be assigned a `tag` from the workflow's groups. During execution, an agent can call `nrflo skip <tag>` to add a tag to the instance's skip list. The orchestrator checks skip tags before each layer to skip agents whose tag is in the list.

### Scope Types

| Scope | Ticket required | Git worktree | Concurrent instances |
|-------|-----------------|--------------|---------------------|
| `ticket` | Yes | Yes | One per ticket+workflow |
| `project` | No | No (runs in project root) | Multiple allowed |

**Project workflow notes:** Project-level workflows are typically used for tasks that don't modify project files — for example, ticket management, project analysis, or cross-cutting coordination. Because they run directly in the project root (not a git worktree), the automatic merge-on-completion behavior does not apply. If a project workflow agent does modify files, those changes remain as uncommitted changes in the project directory.

---

## 8. Callback Mechanism

Allows a later-layer agent (e.g., qa-verifier) to trigger re-execution of an earlier layer.

### Flow

1. Verifier agent detects an issue
2. Verifier saves callback instructions as a finding:
   ```bash
   nrflo findings add callback_instructions:"Fix the auth bug in middleware/auth.go"
   ```
3. Verifier triggers callback:
   ```bash
   nrflo agent callback --level 2
   ```
4. The system resets phases/sessions from the target layer forward
5. Target agent (implementor at layer 2) re-runs with `${CALLBACK_INSTRUCTIONS}` expanded
6. After the target layer completes successfully, callback metadata is cleared

### Limits

- Maximum **3 callbacks** per workflow run
- All layers between the calling layer and target layer are reset

---

## 9. Low-Context Continuation

When an agent's remaining context drops below the threshold, the system automatically saves progress and relaunches with a fresh context window.

### How It Works

1. System detects context usage exceeds threshold (default: 75% used, i.e., 25% remaining)
2. Kills the running agent and resumes the session with instructions to save progress to the `to_resume` finding
3. Agent calls `nrflo agent continue` after saving
4. System launches a fresh agent with `${PREVIOUS_DATA}` populated from the `to_resume` finding

### Configuration

- `restart_threshold` in agent definition: percentage of context **remaining** that triggers save (default `25`)
- Lower values = more aggressive (agent uses more context before save)

### Agent Template Pattern

```markdown
## Previous Progress
${PREVIOUS_DATA}

## Your Task
Continue implementation from where the previous session left off.
```

---

## 10. Automatic Failure Restart

When `max_fail_restarts > 0`, a failed agent (non-zero exit or explicit `agent fail`) is automatically restarted up to `max_fail_restarts` times before the failure is final. Unlike low-context continuation, the agent starts completely fresh (no `${PREVIOUS_DATA}`).

### How It Works

1. Agent exits with non-zero code or calls `agent fail`
2. System checks remaining restart budget
3. If restarts remain: relaunches fresh
4. If exhausted: failure propagates normally

Failure restarts are tracked independently from low-context restarts, so both mechanisms can coexist.

---

## 11. Stall Detection & Auto-Restart

The system monitors agent output and automatically restarts frozen agents. Two stall types are detected:

- **Start stall**: Agent produces no output at all within the start timeout (configurable via `stall_start_timeout_sec`)
- **Running stall**: Agent was active but stopped producing output for the running timeout (configurable via `stall_running_timeout_sec`)

### How It Works

1. System monitors time since last agent output
2. On stall: kills agent immediately (no context save — agent is frozen) and relaunches fresh
3. Stall restarts are capped at a fixed budget to prevent infinite loops

### Configuration

- `stall_start_timeout_sec`: `0` = disabled, positive integer = custom seconds
- `stall_running_timeout_sec`: `0` = disabled, positive integer = custom seconds
- Stall restarts are tracked independently from failure restarts and low-context restarts

---

## 12. Common Patterns & Examples

### Example 1: Setup Analyzer Prompt

```markdown
You are a setup analyzer for ticket ${TICKET_ID}.

## Ticket
- **Title:** ${TICKET_TITLE}
- **Description:** ${TICKET_DESCRIPTION}

## Project Context
#{PROJECT_FINDINGS:architecture,conventions}

## Your Task

Analyze the ticket and codebase. Store your findings:

- `summary` — Brief analysis of what needs to be done
- `files_to_modify` — JSON array of file paths
- `implementation_plan` — Step-by-step plan

When done, save your findings and exit cleanly (exit 0 = pass):
nrflo findings add summary:'...' files_to_modify:'[...]' implementation_plan:'...'
```

### Example 2: Implementor with Findings Injection and Callbacks

```markdown
Implement changes for ticket ${TICKET_ID} in the ${WORKFLOW} workflow.

## Previous Analysis
#{FINDINGS:setup-analyzer}

## Test Specifications
#{FINDINGS:test-writer:test_cases,coverage_plan}

## Your Task

Implement the changes described in the analysis. Follow the test specifications.

When done, save your findings and exit cleanly (exit 0 = pass):
nrflo findings add be_changes_summary:'...' be_files_changed:'[...]'

If blocked, fail with a reason:
nrflo agent fail --reason "..."
```

---

## 13. Ticket Management CLI Commands

Use the `nrflo tickets` CLI — **never use `curl` or direct HTTP API calls**.
Requires `NRFLO_PROJECT` env var (already set in spawned sessions).

```bash
# List tickets
nrflo tickets list
nrflo tickets list --status open --type task --parent EPIC-1

# Get a ticket
nrflo tickets get TICKET-1

# Create a ticket
nrflo tickets create --title "My task" [--id MY-ID] [--description "..."] \
  [--type task|bug|epic|story] [--priority 1-4] [--parent PARENT-ID]

# Update ticket fields (only specified flags are changed)
nrflo tickets update TICKET-1 --title "New title"
nrflo tickets update TICKET-1 --parent EPIC-1       # set parent
nrflo tickets update TICKET-1 --parent ""           # clear parent
nrflo tickets update TICKET-1 --priority 2 --type bug

# Close / reopen
nrflo tickets close TICKET-1 [--reason "Done"]
nrflo tickets reopen TICKET-1

# Dependency management
nrflo deps list TICKET-1
nrflo deps add TICKET-1 BLOCKER-1      # TICKET-1 is blocked by BLOCKER-1
nrflo deps remove TICKET-1 BLOCKER-1
```

---

## 14. System Agents

System agents are global agent definitions not tied to any specific project or workflow. They are managed on the **Settings** page. System agents are used for system-level tasks like automatic merge conflict resolution.

---

## 15. How to Update This Document

- This file is `agent_manual.md` in the project root
- Served by `GET /api/v1/docs/agent-manual`
- Rendered via ReactMarkdown on the `/documentation` page in the web UI
- Edit the markdown file directly — changes are picked up on next page load (no cache, read from filesystem)
- Keep it user-focused: explain what things do, not how they're implemented internally
- When backend changes affect user-visible behavior (new template variables, new CLI commands, new agent definition fields), update this doc
