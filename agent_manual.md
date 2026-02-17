# Agent Definition Manual

> Last updated: 2026-02-17

Cheat-sheet for creating agent definitions. Agent definitions are stored in the `agent_definitions` table and managed via `/api/v1/workflows/{wid}/agents` API or the Workflows page in the web UI.

---

## 1. Quick Reference

### Template Variables

| Variable | Description | Scope | Default/Fallback |
|----------|-------------|-------|------------------|
| `${AGENT}` | Agent type ID (e.g., `implementor`) | Both | Always set |
| `${TICKET_ID}` | Current ticket ID | Ticket | Empty for project scope |
| `${TICKET_TITLE}` | Ticket title from DB | Ticket | Empty for project scope |
| `${TICKET_DESCRIPTION}` | Ticket description from DB | Ticket | Empty for project scope |
| `${PROJECT_ID}` | Project identifier | Both | Always set |
| `${WORKFLOW}` | Workflow name (e.g., `feature`) | Both | Always set |
| `${USER_INSTRUCTIONS}` | User instructions from workflow run | Both | `_No user instructions provided_` |
| `${CALLBACK_INSTRUCTIONS}` | Callback context on re-run | Both | `_No callback instructions_` |
| `${PREVIOUS_DATA}` | Saved progress from continued session | Both | Empty string |
| `${PARENT_SESSION}` | Parent orchestration session UUID | Both | Always set |
| `${CHILD_SESSION}` | This agent's session UUID | Both | Always set |
| `${MODEL_ID}` | Full model ID (e.g., `claude:opus`) | Both | Always set |
| `${MODEL}` | Model name (e.g., `opus`) | Both | `sonnet` if empty |

### Findings Patterns

| Pattern | Description |
|---------|-------------|
| `#{FINDINGS:agent}` | All findings from agent |
| `#{FINDINGS:agent:key}` | Single key from agent |
| `#{FINDINGS:agent:key1,key2}` | Multiple keys from agent |
| `#{PROJECT_FINDINGS:key}` | Single project finding |
| `#{PROJECT_FINDINGS:k1,k2}` | Multiple project findings |

---

## 2. Template Variables

### Simple Variables (always available)

```markdown
You are the ${AGENT} agent running in workflow ${WORKFLOW}.
Your session: ${CHILD_SESSION} (parent: ${PARENT_SESSION})
Model: ${MODEL} (${MODEL_ID})
Project: ${PROJECT_ID}
```

### Ticket Context (ticket-scoped workflows only)

```markdown
## Ticket
- **ID:** ${TICKET_ID}
- **Title:** ${TICKET_TITLE}
- **Description:** ${TICKET_DESCRIPTION}
```

For project-scoped workflows, `${TICKET_ID}` is empty, and `${TICKET_TITLE}`/`${TICKET_DESCRIPTION}` resolve to empty strings. Validation at workflow creation rejects project-scoped agent prompts that use these variables.

### User Instructions

```markdown
## User Instructions
${USER_INSTRUCTIONS}
```

Resolves to `_No user instructions provided_` when no instructions were given at workflow launch.

### Callback Instructions

```markdown
## Callback Context
${CALLBACK_INSTRUCTIONS}
```

During normal execution: `_No callback instructions_`

During a callback re-run, expands to:

```
## Callback Instructions

This agent is being re-run due to a callback from a later stage.

Callback triggered by: qa-verifier

<instructions from the calling agent>
```

### Previous Data (Low-Context Continuation)

```markdown
## Previous Progress
${PREVIOUS_DATA}
```

Empty on first run. On continuation, expands to:

```
This is a continuation of a previous run. Here is what was completed:
<contents of to_resume finding from continued session>
```

---

## 3. Findings Patterns

### Agent Findings (`#{FINDINGS:...}`)

Pull prior agent findings into prompts. Expanded after variable substitution.

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

Pull project-level findings (stored separately in the `project_findings` table).

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

## 4. Agent Lifecycle Commands

Spawned agents report results via CLI over Unix socket.

```bash
# Mark agent as completed successfully
nrworkflow agent complete <ticket> <agent-type> -w <workflow> [--model <model>]

# Mark agent as failed
nrworkflow agent fail <ticket> <agent-type> -w <workflow> [--model <model>] [--reason <text>]

# Signal context exhaustion — triggers relaunch with fresh context
nrworkflow agent continue <ticket> <agent-type> -w <workflow> [--model <model>]

# Callback to re-run an earlier layer
nrworkflow agent callback <ticket> <agent-type> -w <workflow> --level <N> [--model <model>]

# Project-scoped (no ticket): use -T instead of <ticket>
nrworkflow agent complete -T <agent-type> -w <workflow> [--model <model>]
```

| Command | When to use |
|---------|------------|
| `complete` | Task finished successfully |
| `fail` | Task cannot be completed; `--reason` is optional but recommended |
| `continue` | Context window exhausted; save progress to `to_resume` first |
| `callback` | Issue found that requires re-running an earlier layer; `--level` (0-based layer index) is required |

All commands require `-w/--workflow`. The `--model` flag is only needed for parallel agents (multiple models in the same layer). Use `-T/--no-ticket` for project-scoped workflows (skips the `<ticket>` positional arg).

---

## 5. Findings CLI Commands

### Agent-Level Findings

```bash
# Add single finding
nrworkflow findings add <ticket> <agent-type> <key> <value> -w <workflow> [--model <model>]

# Add multiple findings (batch syntax)
nrworkflow findings add <ticket> <agent-type> key1:val1 key2:val2 -w <workflow> [--model <model>]

# Append to existing finding (creates array if needed)
nrworkflow findings append <ticket> <agent-type> <key> <value> -w <workflow> [--model <model>]
nrworkflow findings append <ticket> <agent-type> key1:val1 key2:val2 -w <workflow>

# Get findings
nrworkflow findings get <ticket> <agent-type> -w <workflow>              # all findings
nrworkflow findings get <ticket> <agent-type> <key> -w <workflow>        # single key (positional)
nrworkflow findings get <ticket> <agent-type> -k key1 -k key2 -w <workflow>  # multiple keys

# Delete findings
nrworkflow findings delete <ticket> <agent-type> <key1> [key2...] -w <workflow> [--model <model>]

# Project-scoped (no ticket): use -T instead of <ticket>
nrworkflow findings add -T <agent-type> key1:val1 -w <workflow> [--model <model>]
nrworkflow findings get -T <agent-type> -w <workflow> -k summary
```

### Project-Level Findings

Project findings have no `-w`, `--model`, ticket, or agent-type parameters.

```bash
# Add
nrworkflow findings project-add <key> <value>
nrworkflow findings project-add key1:val1 key2:val2

# Get
nrworkflow findings project-get                    # all project findings
nrworkflow findings project-get <key>              # single key
nrworkflow findings project-get -k key1 -k key2    # multiple keys

# Append
nrworkflow findings project-append <key> <value>
nrworkflow findings project-append key1:val1 key2:val2

# Delete
nrworkflow findings project-delete <key1> [key2...]
```

### Batch Syntax

Both `add` and `append` support `key:value` pairs. The first colon separates the key from the value:

```bash
nrworkflow findings add TICKET-1 implementor summary:'Fixed the auth bug' files_changed:'["auth.go"]' -w bugfix
```

---

## 6. Agent Definition Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `id` | string | Required | Agent type identifier (e.g., `setup-analyzer`, `implementor`) |
| `model` | string | `sonnet` | Model to use (see table below) |
| `timeout` | int | `20` | Max execution time in minutes |
| `prompt` | string | Required | Prompt template with `${VAR}` and `#{FINDINGS:...}` patterns |
| `restart_threshold` | int (optional) | `25` | Percentage of context remaining that triggers low-context save (lower = more aggressive) |

### Supported Models by CLI Adapter

**Claude (`claude` CLI):**

| Model value | Maps to |
|-------------|---------|
| `opus` | Claude Opus |
| `sonnet` | Claude Sonnet |
| `haiku` | Claude Haiku |

**Opencode (`opencode` CLI):**

| Model value | Maps to |
|-------------|---------|
| `opus` | `anthropic/claude-opus-4-5` |
| `sonnet` | `anthropic/claude-sonnet-4-5` |
| `haiku` | `anthropic/claude-haiku-4-5` |
| `gpt_5.3` | `openai/gpt-5.3-codex` with `--variant xhigh` |
| `gpt_max` | `openai/gpt-5.2-codex` with `--variant max` |
| `gpt_high` | `openai/gpt-5.2-codex` with `--variant high` |
| `gpt_medium` | `openai/gpt-5.2-codex` with `--variant medium` |
| `gpt_low` | `openai/gpt-5.2-codex` with `--variant low` |

**Codex (`codex` CLI):**

| Model value | Maps to |
|-------------|---------|
| `gpt_5.3` | `gpt-5.3` reasoning effort "xhigh" |
| `gpt_xhigh` | `gpt-5.2-codex` reasoning effort "xhigh" |
| `gpt_high` / `opus` | `gpt-5.2-codex` reasoning effort "high" |
| `gpt_medium` / `sonnet` / `haiku` | `gpt-5.2-codex` reasoning effort "medium" |

> Check `be/internal/spawner/CLAUDE.md` for the latest model mapping if new adapters are added.

---

## 7. Workflow & Phase Configuration

### Phase JSON Format

Each phase is a JSON object with `agent` (agent definition ID) and `layer` (integer >= 0):

```json
[
  {"agent": "setup-analyzer", "layer": 0},
  {"agent": "test-writer", "layer": 1},
  {"agent": "implementor", "layer": 2},
  {"agent": "qa-verifier", "layer": 3}
]
```

### Layer Execution Rules

- **Concurrent execution:** All agents in the same layer run concurrently
- **Sequential layers:** Layers execute in ascending order (0, 1, 2, ...)
- **Fan-in validation:** If layer N has >1 agent, layer N+1 must have exactly 1 agent
- **Pass condition:** At least 1 agent in a layer must pass (`pass_count >= 1`) for the workflow to proceed
- **All skipped:** If all agents in a layer are skipped, the workflow continues

### Scope Types

| Scope | Ticket required | Git worktree | Concurrent instances |
|-------|-----------------|--------------|---------------------|
| `ticket` | Yes | Yes | One per ticket+workflow |
| `project` | No | No (runs in project root) | Multiple allowed |

---

## 8. Callback Mechanism

Allows a later-layer agent (e.g., qa-verifier) to trigger re-execution of an earlier layer.

### Flow

1. Verifier agent detects an issue
2. Verifier saves callback instructions as a finding:
   ```bash
   nrworkflow findings add TICKET-1 qa-verifier callback_instructions:"Fix the auth bug in middleware/auth.go" -w feature
   ```
3. Verifier triggers callback:
   ```bash
   nrworkflow agent callback TICKET-1 qa-verifier -w feature --level 2
   ```
4. Orchestrator saves `_callback` metadata, resets phases/sessions from target layer forward
5. Target agent (implementor at layer 2) re-runs with `${CALLBACK_INSTRUCTIONS}` expanded
6. After target layer completes successfully, `_callback` metadata is cleared

### Limits

- Maximum **3 callbacks** per workflow run
- All layers between the calling layer and target layer are reset

---

## 9. Low-Context Continuation

When an agent's remaining context drops below the threshold, the spawner automatically saves progress and relaunches.

### How It Works

1. Spawner detects context usage exceeds threshold (default: 75% used, i.e., 25% remaining)
2. Kills the running agent (SIGTERM, then SIGKILL after 5s grace period)
3. If CLI supports resume: resumes session with instructions to save progress to `to_resume` finding
4. Agent calls `nrworkflow agent continue` after saving
5. Spawner launches a fresh agent with `${PREVIOUS_DATA}` populated from the `to_resume` finding
6. Old session gets `status='continued'` and `ancestor_session_id` links the chain

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

## 10. Common Patterns & Examples

### Example 1: Setup Analyzer Prompt

```markdown
You are a setup analyzer for ticket ${TICKET_ID}.

## Ticket
- **Title:** ${TICKET_TITLE}
- **Description:** ${TICKET_DESCRIPTION}

## User Instructions
${USER_INSTRUCTIONS}

## Project Context
#{PROJECT_FINDINGS:architecture,conventions}

## Your Task

Analyze the ticket and codebase. Store your findings:

- `summary` — Brief analysis of what needs to be done
- `files_to_modify` — JSON array of file paths
- `implementation_plan` — Step-by-step plan

When done (ticket-scoped):
nrworkflow findings add ${TICKET_ID} ${AGENT} summary:'...' files_to_modify:'[...]' implementation_plan:'...' -w ${WORKFLOW} --model ${MODEL}
nrworkflow agent complete ${TICKET_ID} ${AGENT} -w ${WORKFLOW} --model ${MODEL}

When done (project-scoped, use -T):
nrworkflow findings add -T ${AGENT} summary:'...' files_to_modify:'[...]' implementation_plan:'...' -w ${WORKFLOW} --model ${MODEL}
nrworkflow agent complete -T ${AGENT} -w ${WORKFLOW} --model ${MODEL}
```

### Example 2: Implementor with Findings Injection and Callbacks

```markdown
You are the ${AGENT} agent for ticket ${TICKET_ID}.

## Previous Analysis
#{FINDINGS:setup-analyzer}

## Test Specifications
#{FINDINGS:test-writer:test_cases,coverage_plan}

## Callback Context
${CALLBACK_INSTRUCTIONS}

## Previous Progress
${PREVIOUS_DATA}

## Your Task

Implement the changes described in the analysis. Follow the test specifications.

When done:
nrworkflow findings add ${TICKET_ID} ${AGENT} be_changes_summary:'...' be_files_changed:'[...]' -w ${WORKFLOW} --model ${MODEL}
nrworkflow agent complete ${TICKET_ID} ${AGENT} -w ${WORKFLOW} --model ${MODEL}

If blocked, fail with a reason:
nrworkflow agent fail ${TICKET_ID} ${AGENT} -w ${WORKFLOW} --model ${MODEL} --reason "..."
```
