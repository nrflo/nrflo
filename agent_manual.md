# Agent Definition Manual

> Last updated: 2026-03-01

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

Spawned agents report results via CLI over Unix socket. **Exiting with code 0 is an implicit pass** — no explicit completion call needed. Only call `agent fail` when the task cannot be completed.

```bash
# Mark agent as failed
nrworkflow agent fail [--reason <text>]

# Signal context exhaustion — triggers relaunch with fresh context
nrworkflow agent continue

# Callback to re-run an earlier layer
nrworkflow agent callback --level <N>

# Skip a workflow group tag (reads NRWF_WORKFLOW_INSTANCE_ID from env)
nrworkflow skip <tag>
```

All context (project, ticket, workflow, agent_type) is derived server-side from `NRWF_SESSION_ID` and `NRWF_WORKFLOW_INSTANCE_ID` env vars set by the spawner. No positional args or `-w`/`-T` flags needed.

| Command | When to use |
|---------|------------|
| `fail` | Task cannot be completed; `--reason` is optional but recommended |
| `continue` | Context window exhausted; save progress to `to_resume` first |
| `callback` | Issue found that requires re-running an earlier layer; `--level` (0-based layer index) is required |
| `skip <tag>` | Skip a workflow group in subsequent layers; tag must be in workflow's `groups` |

**Completion semantics:** Exit 0 = pass (immediate, no grace period). Exit non-zero = fail. Only use `agent fail` for explicit failure with a reason.

---

## 5. Findings CLI Commands

### Agent-Level Findings

```bash
# Add single finding (own session)
nrworkflow findings add <key> <value>

# Add multiple findings (batch syntax)
nrworkflow findings add key1:val1 key2:val2

# Append to existing finding (creates array if needed)
nrworkflow findings append <key> <value>
nrworkflow findings append key1:val1 key2:val2

# Get own findings
nrworkflow findings get                      # all own findings
nrworkflow findings get <key>               # single key (positional)
nrworkflow findings get -k key1 -k key2    # multiple keys

# Get another agent's findings (cross-agent read)
nrworkflow findings get <agent-type>             # all findings for agent
nrworkflow findings get <agent-type> <key>      # single key
nrworkflow findings get <agent-type> -k key1    # multiple keys

# Delete findings
nrworkflow findings delete <key1> [key2...]
```

All context is derived from `NRWF_SESSION_ID` and `NRWF_WORKFLOW_INSTANCE_ID` env vars. Cross-agent reads require providing the target `<agent-type>` and use `NRWF_WORKFLOW_INSTANCE_ID` to scope the lookup.

### Project-Level Findings

Project findings have no ticket or agent-type parameters.

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
nrworkflow findings add summary:'Fixed the auth bug' files_changed:'["auth.go"]'
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
| `max_fail_restarts` | int (optional) | `0` | Max auto-restarts on agent failure (0 = disabled). Failed agent is relaunched fresh (no context save). |
| `stall_start_timeout_sec` | int (optional) | `120` | Seconds with no output before start-stall restart. `0` = disabled. `NULL` = default (120s). |
| `stall_running_timeout_sec` | int (optional) | `480` | Seconds of silence mid-execution before running-stall restart. `0` = disabled. `NULL` = default (480s). |
| `tag` | string (optional) | `""` | Group tag for skip-tag feature; must be in parent workflow's `groups` |

### Supported Models by CLI Adapter

**Claude (`claude` CLI):**

| Model value | Maps to |
|-------------|---------|
| `opus` | Claude Opus (200k context) |
| `opus_1m` | Claude Opus (1M context) |
| `sonnet` | Claude Sonnet |
| `haiku` | Claude Haiku |

**Opencode (`opencode` CLI):**

| Model value | Maps to |
|-------------|---------|
| `opencode_gpt_normal` | `openai/gpt-5.3-codex` with `--variant high` |
| `opencode_gpt_high` | `openai/gpt-5.3-codex` with `--variant high` |

**Codex (`codex` CLI):**

| Model value | Maps to |
|-------------|---------|
| `codex_gpt_normal` | `gpt-5.3-codex` reasoning effort "high" |
| `codex_gpt_high` | `gpt-5.3-codex` reasoning effort "high" |

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

### Workflow Groups (Skip Tags)

Workflows can define `groups` — an array of strings (e.g., `["be", "fe", "docs"]`). Agents can be assigned a `tag` from the workflow's groups. During execution, an agent can call `nrworkflow skip <tag>` to add a tag to the instance's `skip_tags`. The orchestrator reads `skip_tags` before each layer to skip agents whose tag is in the list.

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
   nrworkflow findings add callback_instructions:"Fix the auth bug in middleware/auth.go"
   ```
3. Verifier triggers callback:
   ```bash
   nrworkflow agent callback --level 2
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

## 10. Automatic Failure Restart

When `max_fail_restarts > 0`, a failed agent (non-zero exit or explicit `agent fail`) is automatically restarted up to `max_fail_restarts` times before the failure is final. Unlike low-context continuation, the agent starts completely fresh (no `${PREVIOUS_DATA}`).

### How It Works

1. Agent exits with non-zero code or calls `agent fail`
2. Spawner checks `failRestartCount < max_fail_restarts`
3. If restarts remain: marks old session as `continued` with `result_reason=fail_restart`, relaunches fresh
4. If exhausted: failure propagates normally

The `failRestartCount` is tracked independently from the low-context `restartCount`, so both mechanisms can coexist.

---

## 11. Stall Detection & Auto-Restart

The spawner monitors time since last agent message and automatically restarts frozen agents. Two stall types are detected:

- **Start stall**: Agent produces no output at all within `stall_start_timeout_sec` (default 120s)
- **Running stall**: Agent was active but stopped producing output for `stall_running_timeout_sec` (default 480s)

### How It Works

1. `lastMessageTime` is updated on every `trackMessage()` call (inside `messagesMutex`)
2. `monitorAll()` checks elapsed time since last message each poll iteration
3. On stall: kills agent immediately (no context save — agent is frozen), marks session as `continued` with `result_reason=stall_restart_start_stall` or `stall_restart_running_stall`, relaunches fresh
4. Stall restarts are capped at `maxStallRestarts` (3) to prevent infinite loops
5. Broadcasts `agent.stall_restart` WS event with stall type and count

### Configuration

- `stall_start_timeout_sec`: NULL = 120s default, 0 = disabled, positive integer = custom seconds
- `stall_running_timeout_sec`: NULL = 480s default, 0 = disabled, positive integer = custom seconds
- `stallRestartCount` is independent of `failRestartCount` and `restartCount` (low-context)

---

## 12. Common Patterns & Examples

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

When done, save your findings and exit cleanly (exit 0 = pass):
nrworkflow findings add summary:'...' files_to_modify:'[...]' implementation_plan:'...'
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

When done, save your findings and exit cleanly (exit 0 = pass):
nrworkflow findings add be_changes_summary:'...' be_files_changed:'[...]'

If blocked, fail with a reason:
nrworkflow agent fail --reason "..."
```

---

## 12. Ticket Management CLI Commands

Use the `nrworkflow tickets` CLI — **never use `curl` or direct HTTP API calls**.
Requires `NRWORKFLOW_PROJECT` env var (already set in spawned sessions).

```bash
# List tickets
nrworkflow tickets list
nrworkflow tickets list --status open --type task --parent EPIC-1

# Get a ticket
nrworkflow tickets get TICKET-1

# Create a ticket
nrworkflow tickets create --title "My task" [--id MY-ID] [--description "..."] \
  [--type task|bug|epic|story] [--priority 1-4] [--parent PARENT-ID]

# Update ticket fields (only specified flags are changed)
nrworkflow tickets update TICKET-1 --title "New title"
nrworkflow tickets update TICKET-1 --parent EPIC-1       # set parent
nrworkflow tickets update TICKET-1 --parent ""           # clear parent
nrworkflow tickets update TICKET-1 --priority 2 --type bug

# Close / reopen
nrworkflow tickets close TICKET-1 [--reason "Done"]
nrworkflow tickets reopen TICKET-1

# Dependency management
nrworkflow deps list TICKET-1
nrworkflow deps add TICKET-1 BLOCKER-1      # TICKET-1 is blocked by BLOCKER-1
nrworkflow deps remove TICKET-1 BLOCKER-1
```
