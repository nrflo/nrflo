# Claude Code Instructions for nrworkflow Backend

Go backend for nrworkflow. Single binary runs as server (`nrworkflow serve`) providing HTTP API + WebSocket for the web UI, plus a Unix socket for agent communication. A minimal CLI subset (`agent complete/fail/continue`, `findings add/append/get/delete`) is available for spawned agents.

## Project Structure

```
be/
├── cmd/nrworkflow/main.go       # Entry point
├── internal/
│   ├── cli/                     # Cobra commands
│   │   ├── root.go              # Root command, global flags, project discovery
│   │   ├── serve.go             # HTTP API server (auto-migrates DB)
│   │   ├── agent.go             # agent complete/fail/continue (agent-only)
│   │   ├── findings.go          # findings add/append/get/delete (agent-only)
│   │   ├── tickets.go           # tickets list/get/create (HTTP)
│   │   ├── tickets_update.go    # tickets update/close/reopen/delete (HTTP)
│   │   └── deps.go              # deps list/add/remove (HTTP)
│   ├── spawner/                 # Agent spawner
│   │   ├── spawner.go           # Spawn and monitor agents
│   │   ├── cli_adapter.go       # CLI adapter pattern (Claude, Opencode, Codex)
│   │   ├── cli_adapter_test.go  # Adapter tests
│   │   ├── completion.go        # Completion handling, continuation relaunch
│   │   ├── context_save.go      # Low-context save: kill, resume, save findings, relaunch
│   │   ├── context.go           # Context tracking from /tmp/usable_context.json
│   │   ├── database.go          # DB operations: register start/stop, phase management
│   │   ├── output.go            # Output monitoring, message formatting
│   │   ├── template.go          # Template loading, variable expansion
│   │   └── template_findings.go # Findings expansion, ${PREVIOUS_DATA}, formatting
│   ├── orchestrator/            # Server-side workflow orchestration
│   │   └── orchestrator.go      # Run workflows from UI (layer-grouped concurrent phases)
│   ├── api/                     # HTTP API
│   │   ├── server.go            # Server setup, CORS, WebSocket hub, orchestrator
│   │   ├── handlers_tickets.go  # Ticket list/create/get endpoints
│   │   ├── handlers_tickets_update.go # Ticket update/delete/close/reopen endpoints
│   │   ├── handlers_workflow.go # Workflow state endpoints
│   │   ├── handlers_orchestrate.go # Orchestration run/stop/restart endpoints
│   │   ├── handlers_workflow_def.go # Workflow definition endpoints
│   │   └── handlers_agent_def.go # Agent definition endpoints
│   ├── ws/                      # WebSocket support
│   │   ├── hub.go               # Client management, broadcasting
│   │   ├── client.go            # Connection handling, subscriptions
│   │   ├── handler.go           # HTTP upgrade handler
│   │   └── testing.go           # Test helpers (NewTestClient)
│   ├── config/                  # Configuration management
│   │   └── config.go
│   ├── client/                  # Socket + HTTP clients
│   │   ├── client.go            # Unix socket client for agents
│   │   ├── http.go              # HTTP client for ticket/deps CLI commands
│   │   └── output.go            # Output formatting
│   ├── socket/                  # Unix socket server
│   │   ├── server.go            # Socket listener
│   │   ├── handler.go           # Request routing
│   │   └── protocol.go          # JSON-RPC protocol types
│   ├── service/                 # Business logic layer
│   │   ├── project.go           # Project operations
│   │   ├── ticket.go            # Ticket operations
│   │   ├── workflow.go          # Workflow operations
│   │   ├── workflow_defs.go     # Workflow definitions CRUD
│   │   ├── workflow_config.go   # Workflow config loading
│   │   ├── agent.go             # Agent operations
│   │   ├── agent_definition.go  # Agent definition CRUD
│   │   └── findings.go          # Findings operations
│   ├── db/                      # Database layer
│   │   ├── db.go                # SQLite connection
│   │   ├── pool.go              # Connection pool (10 max, 5 idle)
│   │   ├── migrate.go           # Migration runner
│   │   └── migrations/          # SQL files (embedded via //go:embed)
│   │       └── embed.go         # Go embed directive
│   ├── model/                   # Data models
│   │   ├── project.go
│   │   ├── ticket.go
│   │   ├── agent_session.go
│   │   ├── agent_message.go
│   │   ├── agent_definition.go
│   │   ├── workflow.go
│   │   └── workflow_instance.go
│   ├── repo/                    # Repository pattern
│   │   ├── project.go
│   │   ├── ticket.go
│   │   ├── dependency.go
│   │   ├── agent_session.go
│   │   ├── agent_message.go
│   │   ├── agent_definition.go
│   │   ├── workflow.go
│   │   └── workflow_instance.go
│   ├── types/                   # Shared request/response types
│   │   └── request.go
│   ├── integration/             # Integration tests
│   │   ├── testenv.go           # NewTestEnv shared harness
│   │   └── testdata/            # Test config, agent templates
│   └── id/                      # ID generation
│       └── generator.go
├── scripts/
│   ├── test.sh                  # Test runner (flags: -i -v -c -r)
│   └── context-check.sh         # Context usage hook
├── install.sh                  # Installation script
├── go.mod
├── go.sum
└── Makefile
```

## Source File Size Limit

Keep source files under 300 lines. If a newly created or modified file exceeds 300 lines, refactor it by splitting into logical sub-files before committing. This applies to all Go source files (`.go`), test files, and migration scripts.

## Dependencies

- Go 1.25+
- github.com/spf13/cobra - CLI framework
- modernc.org/sqlite - Pure Go SQLite (no CGO)
- github.com/google/uuid - UUID generation
- github.com/gorilla/websocket - WebSocket implementation
- github.com/golang-migrate/migrate - Database migrations

## Building from Source

```bash
cd ~/projects/2026/nrworkflow/be
make build            # Build binary
make build-release    # Optimized build
sudo make install     # Install to /usr/local/bin
make clean            # Clean build artifacts
```

No CGO required (pure Go SQLite via modernc.org/sqlite).

## Server Architecture

`nrworkflow serve` provides:
- **HTTP API** on port 6587 — web UI, REST API, WebSocket
- **Unix socket** at `/tmp/nrworkflow/nrworkflow.sock` — agent communication only (findings, agent completion, ws.broadcast)
- **Auto-migration** — database schema is automatically migrated on startup

The socket uses a JSON-RPC style protocol (line-delimited JSON). Only `findings.*` (add, add-bulk, get, append, append-bulk, delete), `agent.complete/fail/continue`, and `ws.broadcast` methods are supported.

## System Diagrams

### Unix Socket (Agent Communication)

The Unix socket at `/tmp/nrworkflow/nrworkflow.sock` handles agent-facing methods only:

- `findings.*` — add, add-bulk, get, append, append-bulk, delete
- `agent.complete/fail/continue` — mark agent result
- `ws.broadcast` — broadcast events to WebSocket hub

All other operations (tickets, projects, workflows, agents) are managed via the HTTP API.

### CLI Adapter Architecture

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

### Layer-Based Parallel Execution

```
┌─────────────────────────────────────────────────────────────────────┐
│                    LAYER-BASED AGENT EXECUTION                       │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Orchestrator groups phases by layer, executes layers sequentially   │
│       │                                                              │
│       ▼                                                              │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  LAYER 0: [agent-a, agent-b]  (concurrent)                   │    │
│  │    ┌────────────────┐  ┌────────────────┐                    │    │
│  │    │ spawner.Spawn  │  │ spawner.Spawn  │  (one goroutine    │    │
│  │    │ (agent-a)      │  │ (agent-b)      │   per agent)       │    │
│  │    └───────┬────────┘  └───────┬────────┘                    │    │
│  │            └───────────┬───────┘                              │    │
│  │                        ▼                                      │    │
│  │  Fan-in: wait for ALL agents in layer to finish               │    │
│  │    ├── pass_count >= 1 → proceed to next layer               │    │
│  │    ├── all skipped → proceed to next layer                   │    │
│  │    └── pass_count == 0 → fail workflow, stop                 │    │
│  └────────────────────────┬────────────────────────────────────┘    │
│                           ▼                                          │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  LAYER 1: [agent-c]  (single, fan-in convergence)            │    │
│  │    ┌────────────────┐                                        │    │
│  │    │ spawner.Spawn  │                                        │    │
│  │    │ (agent-c)      │                                        │    │
│  │    └───────┬────────┘                                        │    │
│  └────────────┼────────────────────────────────────────────────┘    │
│               ▼                                                      │
│  All layers done → workflow completed                                │
│                                                                      │
│  VALIDATION RULES:                                                   │
│    - layer field required (integer >= 0)                             │
│    - parallel field rejected (breaking change)                       │
│    - fan-in: multi-agent layer → next layer must have 1 agent       │
│    - string-only phase entries rejected                              │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Message Output Format

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
│        "First 300 chars..."                                          │
│        "... [X chars truncated] ..."                                 │
│        "...last 150 chars"                                           │
│                                                                      │
│  Stderr capture:                                                     │
│    [stderr] Error message from CLI                                   │
│                                                                      │
│  Scanner buffer: 10MB limit for large JSON outputs                  │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Database Schema

```
┌─────────────────────────────────────────────────────────────────────┐
│                     DATABASE TABLES                                  │
│              (~/projects/2026/nrworkflow/nrworkflow.data)           │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  CONFIG                                                              │
│    key           TEXT PRIMARY KEY                                    │
│    value         TEXT NOT NULL                                       │
│                                                                      │
│  PROJECTS                                                            │
│    id            TEXT PRIMARY KEY                                    │
│    name          TEXT NOT NULL                                       │
│    root_path     TEXT                                                │
│    default_workflow TEXT                                             │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│                                                                      │
│  TICKETS                                                             │
│    id            TEXT NOT NULL                                       │
│    project_id    TEXT NOT NULL  (FK → projects.id)                  │
│    title         TEXT NOT NULL                                       │
│    description   TEXT                                                │
│    status        TEXT NOT NULL DEFAULT 'open'                        │
│    priority      INTEGER NOT NULL DEFAULT 2                          │
│    issue_type    TEXT NOT NULL DEFAULT 'task'                        │
│    parent_ticket_id TEXT        (optional parent epic reference)     │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│    closed_at     TEXT                                                │
│    created_by    TEXT NOT NULL                                       │
│    close_reason  TEXT                                                │
│    PRIMARY KEY (project_id, id)                                      │
│    INDEX idx_tickets_parent (project_id, parent_ticket_id)           │
│                                                                      │
│  DEPENDENCIES                                                        │
│    project_id     TEXT NOT NULL                                      │
│    issue_id       TEXT NOT NULL                                      │
│    depends_on_id  TEXT NOT NULL                                      │
│    type           TEXT NOT NULL DEFAULT 'blocks'                     │
│    created_at     TEXT NOT NULL                                      │
│    created_by     TEXT NOT NULL                                      │
│    PRIMARY KEY (project_id, issue_id, depends_on_id)                │
│                                                                      │
│  WORKFLOW_INSTANCES                                                  │
│    id              TEXT PRIMARY KEY   (UUID)                         │
│    project_id      TEXT NOT NULL                                     │
│    ticket_id       TEXT NOT NULL                                     │
│    workflow_id     TEXT NOT NULL      (FK → workflows)               │
│    status          TEXT NOT NULL      (active|completed|failed)      │
│    category        TEXT               (full|simple|docs)             │
│    current_phase   TEXT               (currently active phase)       │
│    phase_order     TEXT NOT NULL      (JSON array of phase IDs)      │
│    phases          TEXT NOT NULL      (JSON: {phase: {status,result}})│
│    findings        TEXT NOT NULL      (JSON: workflow-level findings)│
│    retry_count     INTEGER NOT NULL DEFAULT 0                        │
│    parent_session  TEXT               (orchestrating session UUID)   │
│    created_at      TEXT NOT NULL                                     │
│    updated_at      TEXT NOT NULL                                     │
│    UNIQUE (project_id, ticket_id, workflow_id)                       │
│    FK (project_id, workflow_id) → workflows(project_id, id)         │
│    FK (project_id, ticket_id) → tickets(project_id, id) CASCADE     │
│                                                                      │
│  AGENT_SESSIONS                                                      │
│    id            TEXT PRIMARY KEY    (session UUID)                  │
│    project_id    TEXT NOT NULL                                       │
│    ticket_id     TEXT NOT NULL                                       │
│    workflow_instance_id TEXT NOT NULL (FK → workflow_instances.id)   │
│    phase         TEXT NOT NULL       (e.g., "investigation")         │
│    agent_type    TEXT NOT NULL       (e.g., "setup-analyzer")        │
│    model_id      TEXT                (e.g., "claude:sonnet")         │
│    status        TEXT NOT NULL       (running|completed|failed|timeout|continued)
│    result        TEXT                (pass|fail|continue|timeout)    │
│    result_reason TEXT                (explanation for result)        │
│    pid           INTEGER             (OS process ID)                 │
│    findings      TEXT                (JSON: per-agent findings)      │
│    context_left  INTEGER             (remaining context window %)    │
│    ancestor_session_id TEXT          (links continuation chain)      │
│    spawn_command TEXT                (Full CLI command for replay)   │
│    prompt_context TEXT               (System prompt file contents)   │
│    raw_output    TEXT                (Raw stdout/stderr output)      │
│    restart_count INTEGER NOT NULL DEFAULT 0  (low-context restarts) │
│    started_at    TEXT                (when agent started running)    │
│    ended_at      TEXT                (when agent finished)           │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│    FK workflow_instance_id → workflow_instances(id) CASCADE          │
│    FK ancestor_session_id → agent_sessions(id) RESTRICT             │
│                                                                      │
│  AGENT_MESSAGES                                                      │
│    id            INTEGER PRIMARY KEY AUTOINCREMENT                   │
│    session_id    TEXT NOT NULL  (FK → agent_sessions.id, CASCADE)   │
│    seq           INTEGER NOT NULL    (message sequence number)       │
│    content       TEXT NOT NULL       (message text)                  │
│    created_at    TEXT NOT NULL                                       │
│    INDEX idx_agent_messages_session (session_id, seq)                │
│                                                                      │
│  WORKFLOWS                                                           │
│    id            TEXT NOT NULL                                       │
│    project_id    TEXT NOT NULL  (FK → projects.id)                  │
│    description   TEXT                                                │
│    categories    TEXT           (JSON array string)                  │
│    phases        TEXT NOT NULL  (JSON array string)                  │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│    PRIMARY KEY (project_id, id)                                      │
│                                                                      │
│  AGENT_DEFINITIONS                                                   │
│    id            TEXT NOT NULL                                       │
│    project_id    TEXT NOT NULL                                       │
│    workflow_id   TEXT NOT NULL                                       │
│    model         TEXT NOT NULL DEFAULT 'sonnet'                      │
│    timeout       INTEGER NOT NULL DEFAULT 20                         │
│    prompt        TEXT NOT NULL DEFAULT ''                            │
│    restart_threshold INTEGER       (NULL = use global default 25%)   │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│    PRIMARY KEY (project_id, workflow_id, id)                         │
│    FK (project_id, workflow_id) → workflows(project_id, id) CASCADE │
│                                                                      │
│  TICKETS_FTS (Full-text search)                                      │
│    project_id, id, title, description                                │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Agent Spawner

The spawner manages agent lifecycle directly. It reads the workflow's parallel configuration and automatically spawns all configured models for a phase.

### Supported CLIs

The spawner uses a **CLI Adapter pattern** to support multiple AI backends:

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

### Spawn Behavior

The `spawn` command reads the phase's `parallel` configuration:
- **parallel.enabled: true** - Spawns ALL models listed in `parallel.models`
- **parallel.enabled: false** (or not set) - Spawns single agent with default model

Both `--session` and `-w` are **required** for `agent spawn`:
- `--session` must be the parent session's UUID (from `SESSION_MARKER`)
- `-w/--workflow` specifies which workflow to use on the ticket

### Spawn Flow

```
1. VALIDATION
   - Validate workflow is initialized on ticket
   - Check layer ordering (all prior layers must be completed)
   - Check skip_for category rules

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

ORCHESTRATOR LAYER EXECUTION:
   - Groups phases by layer number
   - Spawns all agents in a layer concurrently (goroutine per agent)
   - Waits for all agents in layer to finish before next layer

BROADCAST: The spawner broadcasts WebSocket events (agent.started,
messages.updated, agent.completed, phase.started, phase.completed)
directly via the in-process WebSocket hub.
```

## Agent Definitions

Agent definitions store the model, timeout, and prompt template for each agent type per workflow. They are stored in the `agent_definitions` DB table and managed via:

- **API**: `GET/POST/PATCH/DELETE /api/v1/workflows/{wid}/agents[/{id}]`
- **UI**: Workflows page at `/workflows`

The spawner loads templates exclusively from the DB. Agent definitions must exist in the database before spawning.

| Template | Purpose | Model |
|----------|---------|-------|
| `setup-analyzer` | Investigation and context gathering | sonnet |
| `implementor` | Code implementation | opus |
| `test-writer` | TDD test design | opus |
| `qa-verifier` | Verification and quality checks | opus |
| `doc-updater` | Documentation updates | sonnet |

### Template Variables

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
- `${PREVIOUS_DATA}` - Findings from the most recent continued session (same agent, model, phase). Populated on low-context restarts. Empty string if no prior continued session exists.

Ticket context variables (`${TICKET_TITLE}`, `${TICKET_DESCRIPTION}`, `${USER_INSTRUCTIONS}`) are only fetched from the database when the template contains them, avoiding unnecessary queries.

### Findings Auto-Population

Templates can include findings from previous phases using the `#{FINDINGS:...}` pattern. This eliminates the need for agents to call `nrworkflow findings get` at runtime.

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
  - src/utils.go
patterns:
  validation: use FormValidator
```

**Output format (parallel agents):**
```
- setup-analyzer:claude:opus:
  summary: Analysis found 3 files to modify
  files_to_modify:
    - src/handler.go

- setup-analyzer:claude:sonnet:
  summary: Found pattern in auth module
  files_to_modify:
    - src/auth.go
```

**Missing findings:**
```
_No findings yet available from setup-analyzer_
```

## HTTP API Endpoints

```bash
# Projects
GET /api/v1/projects
GET /api/v1/projects/:id
POST /api/v1/projects
PATCH /api/v1/projects/:id
DELETE /api/v1/projects/:id

# Tickets (require X-Project header or ?project= param)
GET /api/v1/tickets
GET /api/v1/tickets/:id
POST /api/v1/tickets
PATCH /api/v1/tickets/:id
DELETE /api/v1/tickets/:id
POST /api/v1/tickets/:id/close

# Workflow state (ticket-scoped runtime state)
GET /api/v1/tickets/:id/workflow
PATCH /api/v1/tickets/:id/workflow

# Workflow orchestration (run/stop/restart from UI)
POST /api/v1/tickets/:id/workflow/run      # Start orchestrated run
POST /api/v1/tickets/:id/workflow/stop     # Stop running orchestration
POST /api/v1/tickets/:id/workflow/restart  # Restart agent (context save + relaunch)

# Workflow definitions (project-scoped, require X-Project header)
GET    /api/v1/workflows              # List all
POST   /api/v1/workflows              # Create
GET    /api/v1/workflows/:id          # Get one
PATCH  /api/v1/workflows/:id          # Update
DELETE /api/v1/workflows/:id          # Delete

# Agent definitions (nested under workflows)
GET    /api/v1/workflows/:wid/agents           # List agents for workflow
POST   /api/v1/workflows/:wid/agents           # Create agent definition
GET    /api/v1/workflows/:wid/agents/:id       # Get agent definition
PATCH  /api/v1/workflows/:wid/agents/:id       # Update agent definition
DELETE /api/v1/workflows/:wid/agents/:id       # Delete agent definition

# Agent sessions
GET /api/v1/tickets/:id/agents
GET /api/v1/tickets/:id/agents?phase=investigation

# Recent agents (cross-project, no X-Project header required)
GET /api/v1/agents/recent
GET /api/v1/agents/recent?limit=10

# Session messages (paginated, lazy-loaded)
GET /api/v1/sessions/:id/messages
GET /api/v1/sessions/:id/messages?limit=100&offset=0
# Returns: {session_id: string, messages: [{content: string, created_at: string}], total: int}

# Session raw output (raw stdout/stderr)
GET /api/v1/sessions/:id/raw-output

# Dependencies
GET /api/v1/tickets/:id/dependencies  # Get ticket dependencies
POST /api/v1/dependencies             # Add dependency
DELETE /api/v1/dependencies           # Remove dependency

# Other
GET /api/v1/search?q=              # Full-text search
GET /api/v1/status                 # Dashboard summary
GET /api/v1/ws                     # WebSocket for real-time updates
```

Server runs on port 6587 with CORS enabled for `http://localhost:5173`.

## Common Tasks

### Adding a New Agent Type

1. Create agent definition via API: `POST /api/v1/workflows/:wid/agents` with model, timeout, and prompt template
2. Add to workflow phases via API: `PATCH /api/v1/workflows/:id` (or create a new workflow)
3. **Documentation updates:**
   - Root `CLAUDE.md` - add agent to diagrams in System Diagrams section, update file structure, add to Agent Templates table

### Adding a New Workflow

1. Create workflow definition via API: `POST /api/v1/workflows` with phases, description, and categories
2. Ensure all referenced agents have definitions created via `POST /api/v1/workflows/:wid/agents`
3. **Documentation updates:**
   - Root `CLAUDE.md` - add workflow to diagrams in System Diagrams section, add to Workflows table

### Modifying State Structure

1. Update state initialization in `be/internal/service/workflow.go`
2. Update any code reading that state
3. **Documentation updates:**
   - Root `CLAUDE.md` - update state diagrams in System Diagrams section, update State Storage section if user-visible

### Adding a Database Migration

1. Create `be/internal/db/migrations/NNNNNN_description.up.sql` and `.down.sql` (next sequence number, e.g. `000003_add_labels.up.sql`)
2. The up file contains the schema change (e.g. `ALTER TABLE ... ADD COLUMN`)
3. The down file reverses it (e.g. `ALTER TABLE ... DROP COLUMN`)
4. Migrations are embedded automatically via `//go:embed *.sql` in `migrations/embed.go`
5. Rebuild: `cd be && make build`
6. Migrations run automatically on server startup — no manual `migrate` command needed
7. **Documentation updates:**
   - This file (`be/CLAUDE.md`) - update Database Schema section if user-visible

### Changing Agent CLI Commands

The socket only handles agent-facing methods (findings.*, agent.complete/fail/continue, ws.broadcast):

1. Update CLI command in `be/internal/cli/agent.go` or `findings.go`
2. Update socket handler in `be/internal/socket/handler.go`
3. Update service in `be/internal/service/`
4. Rebuild: `cd be && make build`
5. **Documentation updates:**
   - `guidelines/agent-protocol.md` - if agent-facing commands change

### Modifying API Endpoints (HTTP)

1. Update handlers in `be/internal/api/`
2. Update routes in `be/internal/api/server.go`
3. Consider if the same logic should be in socket handler
4. **Documentation updates:**
   - This file (`be/CLAUDE.md`) - update HTTP API Endpoints section
   - `ui/CLAUDE.md` - update API Endpoints section
   - `ui/src/api/` - update corresponding API client
   - `ui/src/types/` - update TypeScript types if needed

## Writing and Verifying Tests

### Using `NewTestEnv(t)`

All integration tests use `NewTestEnv(t)` from `be/internal/integration/testenv.go`. It creates an isolated stack with fresh DB, socket server, WS hub, and seeded project + workflow definition.

Data setup uses services directly (not socket), since the socket only handles agent/findings methods:

```go
func TestSomething(t *testing.T) {
    env := NewTestEnv(t) // fresh DB, socket, hub, project, services

    // Setup via service helpers
    env.CreateTicket(t, "TICKET-1", "Test ticket")
    env.InitWorkflow(t, "TICKET-1")
    env.StartPhase(t, "TICKET-1", "analyzer")

    // Socket calls for agent/findings
    env.MustExecute(t, "findings.add", map[string]interface{}{...}, nil)
    env.MustExecute(t, "agent.complete", map[string]interface{}{...}, nil)

    // WebSocket testing
    _, ch := env.NewWSClient(t, "client-id", "TICKET-1")
}
```

### Helper Methods

| Method | Purpose |
|--------|---------|
| `CreateTicket(t, id, title)` | Create ticket via service |
| `InitWorkflow(t, ticketID)` | Init "test" workflow via service |
| `StartPhase(t, ticketID, phase)` | Start phase via service |
| `CompletePhase(t, ticketID, phase, result)` | Complete phase via service |
| `MustExecute(t, method, params, &result)` | Call socket method (agent/findings only) |
| `ExpectError(t, method, params, code)` | Assert socket error response |
| `NewWSClient(t, id, ticketID)` | Create subscribed WS test client |
| `GetWorkflowInstanceID(t, ticketID, workflow)` | Get workflow instance UUID |
| `InsertAgentSession(t, ...)` | Insert agent session row directly |

Services are also available directly: `env.ProjectSvc`, `env.TicketSvc`, `env.WorkflowSvc`, `env.AgentSvc`, `env.FindingsSvc`.

### Key Gotchas

- **Socket path limit**: macOS has 104-char limit. `NewTestEnv` uses `/tmp/nrwf-it-*.sock`
- **Server stop hangs**: Cleanup uses 2-sec timeout context to avoid blocking
- **No config file needed**: Agent config comes from DB agent_definitions, not file-based config

### Running Tests

```bash
cd be
make test                    # all tests
make test-integration        # integration only (verbose)
./scripts/test.sh -c         # with coverage
./scripts/test.sh -r         # with race detector
./scripts/test.sh -i -v -r   # combine flags
```

### Test Files

| File | Tests |
|------|-------|
| `internal/integration/testenv.go` | Shared test harness (`NewTestEnv`) |
| `internal/integration/workflow_test.go` | Workflow init, phases, set/get (via service) |
| `internal/integration/findings_test.go` | Findings add/append/delete, models (via socket) |
| `internal/integration/agent_test.go` | Agent complete/fail/continue (via socket) |
| `internal/integration/websocket_test.go` | WS broadcast, subscription filtering |
| `internal/integration/messages_test.go` | Agent message storage, pagination (via service) |
| `internal/integration/error_test.go` | Error codes, validation |
| `internal/ws/hub_test.go` | WS hub unit tests |
| `internal/spawner/cli_adapter_test.go` | CLI adapter tests |

### When to Add Tests

- Agent/findings socket changes → add integration test in appropriate `*_test.go`
- Service logic changes → verify existing tests still pass, add cases for new behavior
- Bug fixes → add regression test
