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
│  │   ├── Model: provider/model (anthropic/claude-opus-4-5)     │    │
│  │   │   ├── Auto-maps: opus → anthropic/claude-opus-4-5       │    │
│  │   │   └── GPT aliases: gpt_high → openai/gpt-5.2-codex      │    │
│  │   ├── Reasoning: --variant (max, high, medium, low)         │    │
│  │   │   └── gpt_max → max, gpt_high → high, etc.              │    │
│  │   ├── SessionID: ✗ (generates own)                          │    │
│  │   ├── SystemPromptFile: ✗ (prompt passed inline)            │    │
│  │   └── Resume: ✗                                             │    │
│  └─────────────────────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ CodexAdapter                                                 │    │
│  │   ├── Name: "codex"                                         │    │
│  │   ├── Model: gpt-5.2-codex with reasoning effort levels     │    │
│  │   │   └── gpt_high → high, gpt_xhigh → xhigh, etc.          │    │
│  │   ├── SessionID: ✗ (generates own)                          │    │
│  │   ├── SystemPromptFile: ✗ (prompt passed inline)            │    │
│  │   └── Resume: ✗                                             │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  Usage in spawner:                                                   │
│    adapter, _ := GetCLIAdapter(cliName)  // "claude", "opencode", or "codex"
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

**Model mapping for opencode:**
- `opus` → `anthropic/claude-opus-4-5`
- `sonnet` → `anthropic/claude-sonnet-4-5`
- `haiku` → `anthropic/claude-haiku-4-5`
- `gpt_max` → `openai/gpt-5.2-codex` with `--variant max`
- `gpt_high` → `openai/gpt-5.2-codex` with `--variant high`
- `gpt_medium` → `openai/gpt-5.2-codex` with `--variant medium`
- `gpt_low` → `openai/gpt-5.2-codex` with `--variant low`
- Full format (`openai/gpt-5.2`) → passed as-is (no variant)

**Model mapping for codex:**
- `gpt_xhigh` → `gpt-5.2-codex` with reasoning effort "xhigh"
- `gpt_high` → `gpt-5.2-codex` with reasoning effort "high"
- `opus` → `gpt-5.2-codex` with reasoning effort "high"
- `gpt_medium` → `gpt-5.2-codex` with reasoning effort "medium"
- `sonnet` → `gpt-5.2-codex` with reasoning effort "medium"
- `haiku` → `gpt-5.2-codex` with reasoning effort "medium"
- Custom model names → passed as-is with reasoning effort "medium"

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
   - Broadcast messages.updated every ~2s via WebSocket hub

5. FINALIZE PHASE
   - pass_count >= 1 → layer passes (fan-in)
   - all skipped → layer passes
   - pass_count == 0 → layer fails
   - Call WorkflowService.CompletePhase() directly (in-process)

BROADCAST: The spawner broadcasts WebSocket events (agent.started,
messages.updated, agent.completed, phase.started, phase.completed)
directly via the in-process WebSocket hub.
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

Ticket context variables (`${TICKET_TITLE}`, `${TICKET_DESCRIPTION}`, `${USER_INSTRUCTIONS}`) are only fetched from the database when the template contains them.

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

13 test files in this package:

| File | Tests |
|------|-------|
| `cli_adapter_test.go` | CLI adapter unit tests |

Additional spawner behavior is covered by integration tests in `internal/integration/`.
