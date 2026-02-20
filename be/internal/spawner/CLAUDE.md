# Spawner Package

The spawner manages agent lifecycle — spawning CLI processes, monitoring output, tracking context usage, and handling completion/continuation/callbacks.

## CLI Adapter Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                      CLI ADAPTER PATTERN                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Interface: CLIAdapter                                               │
│    ├── Name() string                                                │
│    ├── BuildCommand(opts SpawnOptions) *exec.Cmd                    │
│    ├── MapModel(model string) string                                │
│    ├── SupportsSessionID() bool                                     │
│    ├── SupportsSystemPromptFile() bool                              │
│    ├── SupportsResume() bool                                        │
│    ├── UsesStdinPrompt() bool                                       │
│    └── BuildResumeCommand(opts ResumeOptions) *exec.Cmd             │
│                                                                      │
│  Implementations:                                                    │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ ClaudeAdapter                                                │    │
│  │   ├── Name: "claude"                                        │    │
│  │   ├── Model: short names (opus, sonnet, haiku)              │    │
│  │   ├── SessionID: ✓ (--session-id)                           │    │
│  │   ├── SystemPromptFile: ✓ (--append-system-prompt-file)     │    │
│  │   └── Resume: ✓ (--resume <session-id>)                     │    │
│  └─────────────────────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ OpencodeAdapter                                              │    │
│  │   ├── Name: "opencode"                                      │    │
│  │   ├── Model: provider/model format                          │    │
│  │   │   ├── opencode_gpt_normal → openai/gpt-5.3-codex        │    │
│  │   │   └── opencode_gpt_high → openai/gpt-5.3-codex          │    │
│  │   ├── Reasoning: --variant high (both models)               │    │
│  │   ├── SessionID: ✗ (generates own)                          │    │
│  │   ├── SystemPromptFile: ✗                                   │    │
│  │   ├── StdinPrompt: ✓ (prompt piped via stdin)               │    │
│  │   └── Resume: ✗                                             │    │
│  └─────────────────────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ CodexAdapter                                                 │    │
│  │   ├── Name: "codex"                                         │    │
│  │   ├── Model: codex_gpt_normal/high → gpt-5.3-codex          │    │
│  │   │   └── Both use reasoning effort "high"                   │    │
│  │   ├── SessionID: ✗ (generates own)                          │    │
│  │   ├── SystemPromptFile: ✗ (prompt passed inline)            │    │
│  │   └── Resume: ✗                                             │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  DockerCLIAdapter (decorator):                                       │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ DockerCLIAdapter                                              │    │
│  │   ├── Wraps any CLIAdapter, transforms into docker run       │    │
│  │   ├── Container name: nrwf-<sessionID[:12]> (-rsm for resume)│    │
│  │   ├── Image: nrworkflow-agent (--platform linux/arm64)       │    │
│  │   ├── Volume mounts: project dir, ~/.claude, /tmp/nrworkflow │    │
│  │   ├── Env: HOST_UID, HOST_GID + all inner env vars           │    │
│  │   ├── TCP socket: always sets NRWORKFLOW_AGENT_HOST           │    │
│  │   │   (host.docker.internal:6588)                            │    │
│  │   └── All other methods delegated to inner adapter           │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  Usage in spawner:                                                   │
│    adapter, _ := GetCLIAdapter(cliName)  // "claude", "opencode", or "codex"
│    if config.DockerConfig != nil {                                   │
│        adapter = NewDockerCLIAdapter(adapter, *config.DockerConfig)  │
│    }                                                                 │
│    cmd := adapter.BuildCommand(SpawnOptions{...})                   │
│    cmd.Start()                                                       │
│                                                                      │
│  Adding new CLI (e.g., cursor):                                      │
│    1. Create CursorAdapter implementing CLIAdapter                  │
│    2. Register in GetCLIAdapter(): case "cursor": return &Cursor... │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Supported CLIs

| CLI | Adapter | Model Format | Session ID |
|-----|---------|--------------|------------|
| `claude` | `ClaudeAdapter` | Short name (`opus`, `sonnet`) | Supported |
| `opencode` | `OpencodeAdapter` | `provider/model` (auto-mapped) | Generated by CLI |
| `codex` | `CodexAdapter` | Model aliases with reasoning levels | Generated by CLI |
| (any) | `DockerCLIAdapter` | Wraps inner adapter (decorator) | Delegates to inner |

**Model mapping for opencode:**
- `opencode_gpt_normal` → `openai/gpt-5.3-codex` with `--variant high`
- `opencode_gpt_high` → `openai/gpt-5.3-codex` with `--variant high`
- Full format (`openai/gpt-5.3-codex`) → passed as-is (no variant)

**Model mapping for codex:**
- `codex_gpt_normal` → `gpt-5.3-codex` with reasoning effort "high"
- `codex_gpt_high` → `gpt-5.3-codex` with reasoning effort "high"
- Custom model names → passed as-is with reasoning effort "high"

## Database Access

The spawner uses a shared `*db.Pool` from `Config.Pool` for all database operations. The orchestrator creates one pool per workflow run and passes it to all spawners in that run. The `pool()` helper method provides access.

The `Config.Clock` field (`clock.Clock`) provides time for DB timestamps (agent start time, message coalescing). Real-time operations (`time.Since`, poll intervals, grace periods) continue using `time.Now()` directly.

Repos accept `db.Querier` interface (satisfied by both `*db.DB` and `*db.Pool`).

## Spawn Flow

```
1. VALIDATION
   - Validate workflow is initialized on ticket
   - Check layer ordering (all prior layers must be completed)

2. DETERMINE MODEL
   - Read model from agent definition (DB)
   - Format as cli:model (e.g. claude:opus)
   - Each Spawn() call handles exactly one agent

3. START PHASE & SPAWN
   - Call WorkflowService.StartPhase() directly (in-process)
   - Assemble prompt with ${MODEL_ID}, ${MODEL} placeholders
   - Spawn CLI process
   - Register session with pid and model

4. MONITOR (single poll loop per agent)
   - Print status every 30 seconds
   - Check process for completion or timeout
   - Handle completion/timeout
   - Broadcast messages.updated (coalesced to one per session per 2s window)

5. FINALIZE PHASE
   - pass_count >= 1 → layer passes (fan-in)
   - all skipped → layer passes
   - pass_count == 0 → layer fails
   - Call WorkflowService.CompletePhase() directly (in-process)

BROADCAST: The spawner broadcasts WebSocket events (agent.started,
messages.updated, agent.completed, agent.take_control, phase.started,
phase.completed) directly via the in-process WebSocket hub.
messages.updated events are coalesced to one per session per 2s window.

6. TAKE-CONTROL (interactive session)
   - `takeControlCh` receives session ID from orchestrator
   - Kills agent: SIGTERM → grace period → SIGKILL
   - Sets session status to user_interactive
   - Broadcasts agent.take_control event
   - Blocks monitorAll on interactiveWaitCh
   - `CompleteInteractive(sessionID)` closes the channel, unblocking
   - Proc treated as PASS for finalizePhase
   - Only works for CLIs with SupportsResume() == true
```

## Agent Definitions

Agent definitions store model, timeout, and prompt template per agent type per workflow. Stored in `agent_definitions` DB table, managed via API (`/api/v1/workflows/{wid}/agents`) and UI (`/workflows`).

| Template | Purpose | Model |
|----------|---------|-------|
| `setup-analyzer` | Investigation and context gathering | sonnet |
| `implementor` | Code implementation | opus |
| `test-writer` | TDD test design | opus |
| `qa-verifier` | Verification and quality checks | opus |
| `doc-updater` | Documentation updates | sonnet |

## Agent Environment Variables

The spawner sets these env vars on every spawned agent process. Child processes (e.g., `nrworkflow` CLI calls) inherit them.

| Variable | Purpose |
|----------|---------|
| `NRWORKFLOW_PROJECT` | Project ID |
| `NRWF_WORKFLOW_INSTANCE_ID` | Workflow instance UUID — used by CLI to target the correct instance directly (required for findings/agent commands) |
| `NRWF_SESSION_ID` | Agent session UUID — used by CLI to target the correct session directly (required for findings/agent commands) |
| `NRWF_SPAWNED` | Set to `1` to indicate agent was spawned by the orchestrator |
| `NRWF_CONTEXT_THRESHOLD` | Context usage threshold percentage for low-context detection |
| `NRWORKFLOW_AGENT_HOST` | TCP host:port for agent communication (Docker only, set by DockerCLIAdapter) |

On relaunch (continuation), `spawnSingle` is called again, so `NRWF_SESSION_ID` gets the new session's UUID. `NRWF_WORKFLOW_INSTANCE_ID` stays the same. The resume flow (`context_save.go`) reuses `proc.cmd.Env`, preserving the old session ID for the save step, then the fresh spawn gets the new one.

## Template Variables

Templates use placeholders injected by the spawner:
- `${AGENT}` - Agent type (e.g., "setup-analyzer", "implementor")
- `${TICKET_ID}` - Current ticket ID
- `${TICKET_TITLE}` - Ticket title from the tickets table
- `${TICKET_DESCRIPTION}` - Ticket description from the tickets table
- `${USER_INSTRUCTIONS}` - User instructions from workflow_instances.findings["user_instructions"]
- `${PARENT_SESSION}` - Parent session UUID
- `${CHILD_SESSION}` - This agent's session UUID
- `${WORKFLOW}` - Current workflow name (e.g., "feature", "bugfix")
- `${MODEL_ID}` - Full model identifier in cli:model format (e.g., "claude:sonnet")
- `${MODEL}` - Just the model name (e.g., "sonnet")
- `${PREVIOUS_DATA}` - The `to_resume` key from findings of the most recent continued session (same agent, model, phase). Populated on low-context restarts. Empty string if no prior continued session.
- `${CALLBACK_INSTRUCTIONS}` - Callback instructions from `workflow_instances.findings["_callback"]`. Returns `"_No callback instructions_"` when no callback is active.
- `#{PROJECT_FINDINGS:key}` - Single project finding value from `project_findings` table. Returns `"_No project finding for key 'keyname'_"` if missing.
- `#{PROJECT_FINDINGS:k1,k2}` - Multiple project findings as `key: value` lines. Missing keys get individual placeholders.

Ticket context variables (`${TICKET_TITLE}`, `${TICKET_DESCRIPTION}`, `${USER_INSTRUCTIONS}`) are only fetched from the database when the template contains them. Project findings are only queried when `#{PROJECT_FINDINGS:...}` patterns are present.

For project-scoped workflows, `${TICKET_ID}` is empty, and `${TICKET_TITLE}`/`${TICKET_DESCRIPTION}` are replaced with empty strings. Validation at workflow creation rejects project-scoped workflows whose agent prompts use ticket-specific variables.

## Findings Auto-Population

Templates can include findings from previous phases using `#{FINDINGS:...}` pattern.

**Syntax:**
- `#{FINDINGS:agent}` - All findings for agent
- `#{FINDINGS:agent:key}` - Single specific key
- `#{FINDINGS:agent:key1,key2}` - Multiple specific keys

**Example template:**
```markdown
## Prior Context
### Investigation Results
#{FINDINGS:setup-analyzer}
### Test Specifications
#{FINDINGS:test-writer:test_cases,coverage_plan}
```

**Output format (single agent):**
```
summary: Analysis found 3 files to modify
files_to_modify:
  - src/handler.go
```

**Output format (parallel agents):**
```
- setup-analyzer:claude:opus:
  summary: Analysis found 3 files to modify
- setup-analyzer:claude:sonnet:
  summary: Found pattern in auth module
```

**Missing findings:** `_No findings yet available from setup-analyzer_`

### Project Findings

Templates can include project-level findings using `#{PROJECT_FINDINGS:...}` pattern. These are stored in the `project_findings` table (separate from agent-level findings).

**Syntax:**
- `#{PROJECT_FINDINGS:key}` - Single key value
- `#{PROJECT_FINDINGS:k1,k2}` - Multiple keys as `key: value` lines

**Example template:**
```markdown
## Project Context
#{PROJECT_FINDINGS:architecture,conventions}
```

**Missing key:** `_No project finding for key 'architecture'_`

## Message Output Format

```
┌─────────────────────────────────────────────────────────────────────┐
│                    TOOL OUTPUT FORMATTING                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  The spawner parses JSON stream output and formats tool details:    │
│                                                                      │
│  Claude CLI format (type: "assistant"):                             │
│    {"type": "assistant", "message": {"content": [                   │
│      {"type": "tool_use", "name": "Bash",                           │
│       "input": {"command": "git status"}}                           │
│    ]}}                                                               │
│           ↓                                                          │
│    [Bash] git status                                                │
│                                                                      │
│  Opencode format (type: "tool_use"):                                │
│    {"type": "tool_use", "part": {"tool": "read",                    │
│     "state": {"input": {"filePath": "/src/main.ts"}}}}              │
│           ↓                                                          │
│    [Read] /src/main.ts                                              │
│                                                                      │
│  CLI Differences (handled automatically):                            │
│    ├── Tool names: Claude=Bash, Opencode=bash (normalized to Title) │
│    ├── Input location: Claude=part.input, Opencode=part.state.input │
│    ├── Field names: Claude=file_path, Opencode=filePath (both work) │
│    └── Skill field: Claude=skill, Opencode=name (both work)         │
│                                                                      │
│  Tool detail extraction by type:                                     │
│    ├── Bash: input.command                                          │
│    ├── Read/Write/Edit: input.file_path OR input.filePath           │
│    ├── Glob: input.pattern (+ input.path)                           │
│    ├── Grep: input.pattern (+ "in" + input.path)                    │
│    ├── Task: input.subagent_type + input.description                │
│    ├── Skill: input.skill OR input.name + input.args                │
│    ├── WebFetch: input.url                                          │
│    ├── WebSearch: input.query                                       │
│    └── Others: just [ToolName]                                      │
│                                                                      │
│  Text message handling:                                              │
│    ├── Short (≤500 chars): Displayed in full                        │
│    └── Long (>500 chars): Truncated as START...END                  │
│                                                                      │
│  Stderr capture: [stderr] Error message from CLI                     │
│  Scanner buffer: 10MB limit for large JSON outputs                  │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Testing

16 test files in this package:

| File | Tests |
|------|-------|
| `cli_adapter_test.go` | CLI adapter unit tests |
| `docker_adapter_test.go` | Docker CLI adapter decorator tests |
| `template_project_findings_test.go` | Project findings template expansion tests |
| `take_control_test.go` | Take-control channel, interactive wait, WS broadcast tests |

Additional spawner behavior is covered by integration tests in `internal/integration/`.
