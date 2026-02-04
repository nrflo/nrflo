# nrworkflow

Multi-workflow state management for ticket implementation. Supports multiple workflows per ticket, with reusable agents across workflows. A single ticket can have multiple workflows initialized (e.g., running both `feature` and `bugfix` workflows on the same ticket).

**v4** adds support for parallel agents - running 1-3 agents (Claude, OpenAI, Gemini, etc.) simultaneously during a single phase, each identified by `cli:model` format.

## Documentation

| Document | Purpose |
|----------|---------|
| **README.md** | User documentation (this file) |
| [CLAUDE.md](CLAUDE.md) | AI assistant maintenance rules |
| [guidelines/agent-protocol.md](guidelines/agent-protocol.md) | Agent conventions |
| [guidelines/findings-schema.md](guidelines/findings-schema.md) | Findings format |
| [nrworkflow/README.md](nrworkflow/README.md) | Go CLI internal documentation |
| [ui/README.md](ui/README.md) | Web UI documentation |

## Quick Start

```bash
# 1. Build and install the Go binary
cd ~/projects/2026/nrworkflow/nrworkflow
make build
sudo cp nrworkflow /usr/local/bin/
# Or create symlink: sudo ln -s $(pwd)/nrworkflow /usr/local/bin/nrworkflow

# 2. Start the server (REQUIRED - CLI communicates via Unix socket)
nrworkflow serve &
# Server starts:
#   HTTP API:     http://localhost:6587 (for web UI)
#   Unix socket:  /tmp/nrworkflow/nrworkflow.sock (for CLI)

# 3. Create a project
nrworkflow project create myproject --name "My Project"

# 4. Set up project config (in your project directory)
mkdir -p .claude/nrworkflow
echo '{"project": "myproject"}' > .claude/nrworkflow/config.json

# 5. Create and implement a ticket (from project directory)
nrworkflow ticket create --type=feature --title="Add login" -d "## Vision
Add user login.

## Acceptance Criteria
- [ ] Login form works"

# 6. Initialize workflow on ticket (auto-creates ticket if not found)
nrworkflow init MYPROJECT-1 -w feature

# 7. Spawn agents (--session and --workflow are required)
nrworkflow agent spawn setup-analyzer MYPROJECT-1 --session=$SESSION_MARKER -w feature
nrworkflow agent spawn implementor MYPROJECT-1 --session=$SESSION_MARKER -w feature
```

**Note:** The server must be running for CLI commands to work. If you get the error "nrworkflow server is not running", start it with `nrworkflow serve`.

## Architecture

```
~/projects/2026/nrworkflow/             # NRWORKFLOW_HOME (source code)
├── nrworkflow/                         # Go CLI source
│   ├── cmd/nrworkflow/main.go          # Entry point
│   ├── internal/                       # Core packages
│   │   ├── cli/                        # Command handlers (thin clients)
│   │   ├── client/                     # Unix socket client
│   │   ├── socket/                     # Unix socket server
│   │   ├── service/                    # Business logic layer
│   │   ├── spawner/                    # Agent spawner
│   │   ├── db/                         # SQLite database + connection pool
│   │   ├── model/                      # Data models
│   │   ├── repo/                       # Repositories
│   │   ├── api/                        # HTTP API (for web UI)
│   │   └── types/                      # Shared request/response types
│   └── Makefile                        # Build commands
├── nrworkflow.data                     # Global ticket database (SQLite)
└── guidelines/                         # Shared protocols
    ├── findings-schema.md
    └── agent-protocol.md

<project>/.claude/nrworkflow/           # PROJECT (required for commands)
├── config.json                         # Project config (workflows, agents)
└── agents/                             # Agent templates
    ├── setup-analyzer.md
    ├── implementor.md
    ├── test-writer.md
    ├── qa-verifier.md
    └── doc-updater.md
```

## Project-Based Architecture

All tickets are scoped to projects. The project is determined from `.claude/nrworkflow/config.json` in your project directory (searched upward from current directory):

```json
{
  "project": "myproject"
}
```

**Priority order:**
1. `NRWORKFLOW_PROJECT` environment variable (for CI/CD, scripting)
2. `.claude/nrworkflow/config.json` searched upward from current directory

```bash
# From within project directory (uses config.json)
cd /path/to/myproject
nrworkflow ticket list

# Or via environment variable (overrides config)
NRWORKFLOW_PROJECT=myproject nrworkflow ticket list
```

Database location:
- Default: `~/projects/2026/nrworkflow/nrworkflow.data` (single global database)
- Override via env: `NRWORKFLOW_HOME=/path/to/nrworkflow`
- Override via flag: `-D /path/to/db.data`

## System Diagrams

### High-Level Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              USER SESSION                                    │
│                         (SESSION_MARKER=<uuid>)                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                    ┌─────────────────┴─────────────────┐
                    ▼                                   ▼
            ┌───────────────┐                   ┌───────────────┐
            │    /prep      │                   │    /impl      │
            │  (planning)   │                   │ (execution)   │
            └───────┬───────┘                   └───────┬───────┘
                    │                                   │
                    ▼                                   ▼
            ┌───────────────┐                   ┌───────────────────────┐
            │ ticket create │                   │ agent spawn --session │
            │ + init        │                   │ (requires parent UUID)│
            └───────┬───────┘                   └───────────┬───────────┘
                    │                                       │
                    │  Unix Socket                          │
                    │  /tmp/nrworkflow/nrworkflow.sock      │
                    ▼                                       ▼
            ┌───────────────────────────────────────────────────────────┐
            │                  nrworkflow serve                         │
            │  ┌─────────────────────────────────────────────────────┐  │
            │  │              Service Layer                           │  │
            │  │  (ticket, project, workflow, agent, findings)        │  │
            │  └──────────────────────┬──────────────────────────────┘  │
            │                         │                                 │
            │  ┌──────────────────────▼──────────────────────────────┐  │
            │  │           DB Connection Pool (10 max, 5 idle)        │  │
            │  └──────────────────────┬──────────────────────────────┘  │
            │                         │                                 │
            │                         ▼                                 │
            │  ┌─────────────────────────────────────────────────────┐  │
            │  │                  nrworkflow.data                     │  │
            │  │     (~/projects/2026/nrworkflow/nrworkflow.data)     │  │
            │  │  - projects table                                    │  │
            │  │  - tickets table (with project_id)                   │  │
            │  │  - agents_state (nrworkflow state JSON)              │  │
            │  │  - agent_sessions table                              │  │
            │  └─────────────────────────────────────────────────────┘  │
            │                                                           │
            │  Also: HTTP API on :6587 (for web UI)                    │
            └───────────────────────────────────────────────────────────┘
```

### Unix Socket IPC Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     UNIX SOCKET COMMUNICATION                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  CLI Commands (thin clients)                                                 │
│    │                                                                         │
│    ├── nrworkflow ticket list                                               │
│    ├── nrworkflow project create                                            │
│    ├── nrworkflow workflow init                                             │
│    └── etc.                                                                  │
│         │                                                                    │
│         │  Connect to /tmp/nrworkflow/nrworkflow.sock                       │
│         │  Send JSON-RPC request                                            │
│         │                                                                    │
│         ▼                                                                    │
│    ┌───────────────────────────────────────────────────────────────────┐    │
│    │                   JSON-RPC Protocol                                │    │
│    │                                                                    │    │
│    │  Request:                                                          │    │
│    │  {"id":"uuid","method":"ticket.list","params":{},"project":"xyz"} │    │
│    │                                                                    │    │
│    │  Response:                                                         │    │
│    │  {"id":"uuid","result":[{...}],"error":null}                      │    │
│    │                                                                    │    │
│    │  Error:                                                            │    │
│    │  {"id":"uuid","result":null,"error":{"code":-32603,"message":""}} │    │
│    └───────────────────────────────────────────────────────────────────┘    │
│         │                                                                    │
│         ▼                                                                    │
│    ┌───────────────────────────────────────────────────────────────────┐    │
│    │                   nrworkflow serve                                 │    │
│    │                                                                    │    │
│    │  Listens on:                                                       │    │
│    │    - Unix socket: /tmp/nrworkflow/nrworkflow.sock (CLI)           │    │
│    │    - HTTP port: :6587 (Web UI)                                     │    │
│    │                                                                    │    │
│    │  Socket Handler → Service Layer → DB Pool → SQLite                │    │
│    │                                                                    │    │
│    │  Method Routing:                                                   │    │
│    │    ticket.* → TicketService                                        │    │
│    │    project.* → ProjectService                                      │    │
│    │    workflow.* → WorkflowService                                    │    │
│    │    phase.* → WorkflowService                                       │    │
│    │    findings.* → FindingsService                                    │    │
│    │    agent.* → AgentService                                          │    │
│    └───────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Exception: agent spawn/preview run directly (no socket)                    │
│    - These need foreground process management                               │
│    - They read config and spawn processes directly                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Skill Workflows

#### /prep - Planning Skill

```
┌─────────────────────────────────────────────────────────────────────┐
│                           /prep WORKFLOW                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Phase 1: Understand ──► Phase 2: Explore ──► Phase 3: Plan         │
│       │                       │                     │                │
│       ▼                       ▼                     ▼                │
│  Parse request          codebase-explorer     Create feature plan   │
│  Identify scope         Find patterns         User stories          │
│                         Find files            Technical approach    │
│                                                                      │
│                              │                                       │
│                              ▼                                       │
│  Phase 4: Clarify ◄──────────┘                                      │
│       │                                                              │
│       ▼                                                              │
│  AskUserQuestion (resolve ALL questions)                            │
│       │                                                              │
│       ▼                                                              │
│  Phase 5: Iterate ──► "Create tickets?" ──► Phase 6: Epic?         │
│       │                                          │                   │
│       │                    ┌─────────────────────┴──────────────┐   │
│       │                    ▼                                    ▼   │
│       │              Single ticket                        Epic mode │
│       │                    │                         ┌──────────────┤
│       │                    │                         ▼              │
│       │                    │                   single-shot │ separate│
│       │                    │                         │              │
│       ▼                    ▼                         ▼              │
│  Phase 7: Create Tickets ◄───────────────────────────┘              │
│       │                                                              │
│       ▼                                                              │
│  nrworkflow ticket create -p <project> --type=<type> --title="..."  │
│  nrworkflow ticket dep add <child> <parent> -p <project>            │
│       │                                                              │
│       ▼                                                              │
│  Phase 8: Initialize                                                 │
│       │                                                              │
│       ▼                                                              │
│  nrworkflow init <ticket> -p <project> -w <workflow>                │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘

Output: Tickets ready for /impl
```

#### /impl - Implementation Skill

```
┌─────────────────────────────────────────────────────────────────────┐
│                           /impl WORKFLOW                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Phase 1: Selection & State Check                                   │
│       │                                                              │
│       ├── nrworkflow ticket status -p <project>  (dashboard)        │
│       ├── nrworkflow ticket ready -p <project>   (unblocked tickets)│
│       ├── nrworkflow status <ticket> -p <project> (check state)     │
│       └── nrworkflow ticket update <ticket> -p <project> --status=in_progress
│       │                                                              │
│       ▼                                                              │
│  Phase 2: Investigation                                             │
│       │                                                              │
│       └── nrworkflow agent spawn setup-analyzer <ticket> -p <project> --session
│           │                                                          │
│           └── Sets category: docs | simple | full                   │
│       │                                                              │
│       ▼                                                              │
│  Phase 3: Test Design (skip if docs/simple)                         │
│       │                                                              │
│       └── nrworkflow agent spawn test-writer <ticket> -p <project> --session
│       │                                                              │
│       ▼                                                              │
│  Phase 4: Implementation                                            │
│       │                                                              │
│       └── nrworkflow agent spawn implementor <ticket> -p <project> --session
│       │                                                              │
│       ▼                                                              │
│  Phase 5: Verification (skip if docs)                               │
│       │                                                              │
│       └── nrworkflow agent spawn qa-verifier <ticket> -p <project> --session
│           │                                                          │
│           └── On FAIL: nrworkflow agent retry + re-spawn            │
│       │                                                              │
│       ▼                                                              │
│  Phase 6: Documentation                                             │
│       │                                                              │
│       └── nrworkflow agent spawn doc-updater <ticket> -p <project> --session
│       │                                                              │
│       ▼                                                              │
│  Phase 7: Completion                                                │
│       │                                                              │
│       ├── nrworkflow ticket close <ticket> -p <project>             │
│       └── git commit + push                                         │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Session Hierarchy

```
┌─────────────────────────────────────────────────────────────────────┐
│                       SESSION HIERARCHY                              │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  User Shell (wrapper: cld())                                        │
│       │                                                              │
│       └── Generates SESSION_MARKER=<uuid>                           │
│           │                                                          │
│           ▼                                                          │
│  Main Claude Session (parent)                                       │
│       │                                                              │
│       ├── Runs /prep or /impl skill                                 │
│       │                                                              │
│       └── Passes -p, --session, -w, and --model to spawner          │
│           │                                                          │
│           ▼                                                          │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ Spawned Agent (child)                                        │    │
│  │     │                                                        │    │
│  │     ├── parent_session = $SESSION_MARKER (from --session)   │    │
│  │     ├── child_session = <new uuid> (generated by spawner)   │    │
│  │     ├── workflow = <name> (from -w)                         │    │
│  │     ├── model_id = cli:model (from config)                  │    │
│  │     └── PID tracked for kill support                        │    │
│  │                                                              │    │
│  │ Prompt contains:                                             │    │
│  │     ## Parent Session: <parent_uuid>                        │    │
│  │     ## CHILD_SESSION_MARKER=<child_uuid>                    │    │
│  │     ## Model ID: <cli:model>                                │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘

State stored in ticket (v4 format - per workflow):
{
  "feature": {                               ← Workflow name as key
    "version": 4,
    "parent_session": "<parent_uuid>",       ← Saved on first spawn
    "active_agents": {                       ← v4: dict of parallel agents
      "setup-analyzer:claude:sonnet": {
        "agent_id": "spawn-abc123",
        "agent_type": "setup-analyzer",
        "model_id": "claude:sonnet",
        "session_id": "<child_uuid>",
        "pid": 12345,
        "result": null
      },
      "setup-analyzer:opencode:opus": {
        "agent_id": "spawn-def456",
        "model_id": "opencode:opus",
        ...
      }
    }
  },
  "bugfix": {                                ← Another workflow (independent)
    "version": 4,
    ...
  }
}
```

### Parallel Agent Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                    PARALLEL AGENT SPAWNING                           │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Skill/Orchestrator (single spawn command)                          │
│       │                                                              │
│       └── nrworkflow agent spawn setup-analyzer TICKET -p proj --session=...
│           │                                                          │
│           ▼                                                          │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │              SPAWNER READS WORKFLOW CONFIG                    │    │
│  │                                                              │    │
│  │  parallel: {enabled: true, models: ["claude:sonnet",        │    │
│  │                                      "opencode:opus"]}       │    │
│  └──────────────────────────┬──────────────────────────────────┘    │
│                             │                                        │
│                             ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │              SPAWN ALL MODELS IN PARALLEL                     │    │
│  │                                                              │    │
│  │  ┌─────────────────────┐  ┌─────────────────────┐           │    │
│  │  │ Claude CLI (sonnet) │  │ OpenCode CLI (opus) │           │    │
│  │  │  PID: 12345         │  │  PID: 12346         │           │    │
│  │  └──────────┬──────────┘  └──────────┬──────────┘           │    │
│  │             │                        │                       │    │
│  │             └────────────┬───────────┘                       │    │
│  │                          │                                   │    │
│  │                          ▼                                   │    │
│  │              SINGLE MONITOR LOOP                             │    │
│  │              ├── Print status every 30s                      │    │
│  │              ├── Check process completion                    │    │
│  │              ├── Handle timeout (kill + log)                 │    │
│  │              └── Wait until ALL complete                     │    │
│  │                          │                                   │    │
│  └──────────────────────────┼──────────────────────────────────┘    │
│                             │                                        │
│                             ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                 active_agents                                │    │
│  │  {                                                           │    │
│  │    "setup-analyzer:claude:sonnet": {pid, result...},        │    │
│  │    "setup-analyzer:opencode:opus": {pid, result...}         │    │
│  │  }                                                           │    │
│  └──────────────────────────┬──────────────────────────────────┘    │
│                             │                                        │
│                             ▼                                        │
│  PHASE COMPLETION (automatic when all agents done):                 │
│       ├── ALL agents pass → phase passes                            │
│       └── ANY agent fails → phase fails                             │
│                                                                      │
│  Example output:                                                     │
│  ┌────────────────────────────────────────────────────────────┐     │
│  │ [investigation] 2 agent(s) running:                        │     │
│  │   claude:sonnet: 30s | findings: 2                         │     │
│  │   opencode:opus: 32s                                       │     │
│  │                                                            │     │
│  │ [investigation] All agents completed:                      │     │
│  │   claude:sonnet: PASS (95s)                                │     │
│  │   opencode:opus: PASS (120s)                               │     │
│  │                                                            │     │
│  │ Phase complete: investigation (PASS)                       │     │
│  └────────────────────────────────────────────────────────────┘     │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

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
│    ├── SupportsMaxTurns() bool                                      │
│    └── SupportsSystemPromptFile() bool                              │
│                                                                      │
│  Implementations:                                                    │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ ClaudeAdapter                                                │    │
│  │   ├── Name: "claude"                                        │    │
│  │   ├── Model: short names (opus, sonnet, haiku)              │    │
│  │   ├── SessionID: ✓ (--session-id)                           │    │
│  │   ├── MaxTurns: ✓ (--max-turns)                             │    │
│  │   └── SystemPromptFile: ✓ (--append-system-prompt-file)     │    │
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
│  │   ├── MaxTurns: ✗ (runs until done)                         │    │
│  │   └── SystemPromptFile: ✗ (prompt passed inline)            │    │
│  └─────────────────────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ CodexAdapter                                                 │    │
│  │   ├── Name: "codex"                                         │    │
│  │   ├── Model: gpt-5.2-codex with reasoning effort levels     │    │
│  │   │   └── gpt_high → high, gpt_xhigh → xhigh, etc.          │    │
│  │   ├── SessionID: ✗ (generates own)                          │    │
│  │   ├── MaxTurns: ✗ (runs until done)                         │    │
│  │   └── SystemPromptFile: ✗ (prompt passed inline)            │    │
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
│  Example output during agent run:                                    │
│  ┌────────────────────────────────────────────────────────────┐     │
│  │ [Bash] git diff origin/main...HEAD                         │     │
│  │ [Read] /Users/dev/project/src/api/client.ts                │     │
│  │ [Grep] handleError in src/services/                        │     │
│  │ [Skill] skill:jira-ticket REF-12425                        │     │
│  │ [Task] codebase-explorer: Find authentication handlers     │     │
│  │ [Edit] /src/api/client.ts                                  │     │
│  │ [stderr] API rate limit warning                            │     │
│  └────────────────────────────────────────────────────────────┘     │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Ticket State Machine

```
┌─────────────────────────────────────────────────────────────────────┐
│                      TICKET LIFECYCLE                                │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────┐    /prep      ┌─────────────┐                         │
│  │  (none)  │ ───────────► │  CREATED    │                         │
│  └──────────┘               │  (open)     │                         │
│       │                     └──────┬──────┘                         │
│       │                            │ nrworkflow init                │
│       │                            ▼                                 │
│       │                     ┌─────────────┐                         │
│       └────────────────────►│ INITIALIZED │  (init auto-creates     │
│         nrworkflow init     │  (open)     │   ticket if not found)  │
│         (auto-creates)      └──────┬──────┘                         │
│                                    │ /impl starts                   │
│                                    ▼                                 │
│                             ┌─────────────┐                         │
│                             │ IN_PROGRESS │ ◄─────┐                 │
│                             └──────┬──────┘       │                 │
│                                    │              │ retry           │
│       ┌────────────────────────────┼──────────────┤                 │
│       ▼                            ▼              │                 │
│  ┌─────────┐                 ┌─────────┐    ┌─────────┐             │
│  │ BLOCKED │                 │ RUNNING │───►│ FAILED  │             │
│  │(waiting)│                 │ (agent) │    └─────────┘             │
│  └─────────┘                 └────┬────┘                            │
│                                   │ all phases pass                 │
│                                   ▼                                 │
│                             ┌─────────────┐                         │
│                             │  COMPLETED  │                         │
│                             │  (closed)   │                         │
│                             └─────────────┘                         │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Phase State Machine

```
┌─────────────────────────────────────────────────────────────────────┐
│                       PHASE LIFECYCLE                                │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│                             ┌─────────┐                             │
│                             │ pending │                             │
│                             └────┬────┘                             │
│                                  │                                   │
│              ┌───────────────────┼───────────────────┐              │
│              │                   │                   │              │
│              ▼                   ▼                   ▼              │
│        ┌─────────┐        ┌───────────┐       ┌─────────┐          │
│        │ skipped │        │in_progress│       │  N/A    │          │
│        │(skip_for│        └─────┬─────┘       │(previous│          │
│        │ rules)  │              │             │ failed) │          │
│        └─────────┘              │             └─────────┘          │
│                                 │                                   │
│                    ┌────────────┴────────────┐                     │
│                    │                         │                     │
│                    ▼                         ▼                     │
│              ┌───────────┐            ┌───────────┐                │
│              │ completed │            │ completed │                │
│              │  (pass)   │            │  (fail)   │                │
│              └───────────┘            └───────────┘                │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘

Workflow phases (feature workflow):
  investigation ──► test-design ──► implementation ──► verification ──► docs
                    (skip: docs,     (always)         (skip: docs)
                     simple)
```

### Database Schema

```
┌─────────────────────────────────────────────────────────────────────┐
│                     DATABASE TABLES                                  │
│              (~/projects/2026/nrworkflow/nrworkflow.data)           │
├─────────────────────────────────────────────────────────────────────┤
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
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│    closed_at     TEXT                                                │
│    created_by    TEXT NOT NULL                                       │
│    close_reason  TEXT                                                │
│    agents_state  TEXT           (JSON workflow state)                │
│    PRIMARY KEY (project_id, id)                                      │
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
│  AGENT_SESSIONS                                                      │
│    id            TEXT PRIMARY KEY    (session UUID)                  │
│    project_id    TEXT NOT NULL                                       │
│    ticket_id     TEXT NOT NULL                                       │
│    phase         TEXT NOT NULL       (e.g., "investigation")         │
│    workflow      TEXT NOT NULL       (e.g., "feature")               │
│    agent_type    TEXT NOT NULL       (e.g., "setup-analyzer")        │
│    model_id      TEXT                (e.g., "claude:sonnet")         │
│    status        TEXT NOT NULL       (running|completed|failed|timeout)
│    last_messages TEXT                (JSON array of last 50 messages, newest first)
│    message_stats TEXT                (JSON: {"tool:Read": 5, "text": 3})
│    spawn_command TEXT                (Full CLI command for debugging/replay)
│    prompt_context TEXT               (System prompt file contents)   │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
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

| CLI | Adapter | Model Format | Session ID | Max Turns |
|-----|---------|--------------|------------|-----------|
| `claude` | `ClaudeAdapter` | Short name (`opus`, `sonnet`) | Supported | Supported |
| `opencode` | `OpencodeAdapter` | `provider/model` (auto-mapped) | Generated by CLI | Not supported |
| `codex` | `CodexAdapter` | Model aliases with reasoning levels | Generated by CLI | Not supported |

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

```bash
# Spawn agents for a phase (spawns ALL models from workflow config)
nrworkflow agent spawn <agent_type> <ticket_id> --session=<parent_uuid> -w <workflow>

# Preview assembled prompt (debugging)
nrworkflow agent preview <agent_type> <ticket_id> [-w <name>]

# List available agents
nrworkflow agent list
```

Both `--session` and `-w` are **required** for `agent spawn`:
- `--session` must be the parent session's UUID (from `SESSION_MARKER`)
- `-w/--workflow` specifies which workflow to use on the ticket

### Spawn Behavior

The `spawn` command reads the phase's `parallel` configuration:
- **parallel.enabled: true** - Spawns ALL models listed in `parallel.models`
- **parallel.enabled: false** (or not set) - Spawns single agent with default model

```bash
# Workflow with parallel enabled spawns multiple agents automatically
$ nrworkflow agent spawn setup-analyzer TICKET-123 --session=$SESSION -w parallel-test
Spawning setup-analyzer for TICKET-123...
  Parallel mode: 2 models configured
  - claude:sonnet
  - opencode:opus
  Workflow: parallel-test
  Parent session: abc123...

[investigation] 2 agent(s) running:
  claude:sonnet: 30s
  opencode:opus: 32s

[investigation] All agents completed:
  claude:sonnet: PASS (95s)
  opencode:opus: PASS (120s)

Phase complete: investigation (PASS)
```

### Spawn Flow

```
1. VALIDATION
   - Validate workflow is initialized on ticket
     (Error if not: "workflow 'X' not initialized. Run: nrworkflow workflow init ...")
   - Check phase sequence (can't spawn out of order)
   - Check skip_for category rules

2. DETERMINE MODELS
   - Read parallel config from workflow phase
   - If parallel.enabled: use all parallel.models
   - Otherwise: use single default model

3. START PHASE & SPAWN ALL
   - nrworkflow phase start <ticket> <phase> -w <workflow>
   - For each model:
     - Assemble prompt with ${MODEL_ID}, ${MODEL} placeholders
     - Spawn CLI process
     - nrworkflow agent start <ticket> <id> <type> -w <workflow> --pid=<pid> --model=<cli:model>

4. MONITOR ALL (single poll loop)
   - Print status every 30 seconds
   - Check each process for completion or timeout
   - Handle completion/timeout for each agent individually

5. FINALIZE PHASE
   - Phase passes only if ALL agents pass
   - If any fail, phase fails
   - nrworkflow phase complete <ticket> <phase> <result> -w <workflow>
```

## Agent Templates

Located in `<project>/.claude/nrworkflow/agents/`:

| Template | Purpose | Model |
|----------|---------|-------|
| `setup-analyzer.md` | Investigation and context gathering | sonnet |
| `implementor.md` | Code implementation | opus |
| `test-writer.md` | TDD test design | opus |
| `qa-verifier.md` | Verification and quality checks | opus |
| `doc-updater.md` | Documentation updates | sonnet |

**Note:** Templates are required. If a template is missing, the spawner will error with the path where it should be created.

### Template Variables

Templates use placeholders injected by the spawner:
- `${AGENT}` - Agent type (e.g., "setup-analyzer", "implementor")
- `${TICKET_ID}` - Current ticket ID
- `${PARENT_SESSION}` - Parent session UUID
- `${CHILD_SESSION}` - This agent's session UUID
- `${WORKFLOW}` - Current workflow name (e.g., "feature", "bugfix")
- `${MODEL_ID}` - Full model identifier in cli:model format (e.g., "claude:sonnet")
- `${MODEL}` - Just the model name (e.g., "sonnet")

## Workflows

```bash
nrworkflow workflows   # List all workflows
```

| Workflow | Phases | Use Case |
|----------|--------|----------|
| `feature` | investigation -> test-design -> implementation -> verification -> docs | New features (full TDD) |
| `bugfix` | investigation -> implementation -> verification | Bug fixes |
| `hotfix` | implementation | Urgent fixes |
| `docs` | investigation -> docs | Documentation only |
| `refactor` | investigation -> implementation -> verification | Code refactoring |

### Categories

Categories control phase skipping:
- `full` - All phases run
- `simple` - Skip test-design (existing tests cover it)
- `docs` - Skip test-design and verification

Set by setup-analyzer agent:
```bash
nrworkflow set <ticket> category <docs|simple|full> -w <workflow>
```

## Commands

### Project Management

```bash
nrworkflow project create <id> --name "Project Name"
nrworkflow project list
nrworkflow project show <id>
nrworkflow project delete <id>
```

### Agent Management

```bash
# Spawn all agents for a phase (uses workflow's parallel config)
nrworkflow agent spawn <type> <ticket> --session=<uuid> -w <workflow>
nrworkflow agent preview <type> <ticket> [-w <name>]
nrworkflow agent list
nrworkflow agent active <ticket> -w <name>         # List active agents (JSON)
nrworkflow agent kill <ticket> -w <name> [--model=cli:model]  # Kill specific or all
nrworkflow agent retry <ticket> -w <name> [--model=cli:model]
nrworkflow agent complete <ticket> <type> -w <name> [--model=cli:model]
nrworkflow agent fail <ticket> <type> -w <name> [--model=cli:model]
```

#### Parallel Agents

Parallel agent spawning is configured in the workflow definition. A single `spawn` command launches all configured models:

```bash
# Single command spawns all models from parallel config
nrworkflow agent spawn setup-analyzer TICKET-1 --session=$SESSION -w parallel-test
# Output: Spawns claude:sonnet AND opencode:opus (from workflow config)
# Monitors both with status updates every 30 seconds
# Returns when ALL agents complete

# Check if all parallel agents are done (for external monitoring)
nrworkflow phase ready TICKET-1 investigation -w parallel-test
# Output: "ready" or "pending"

# List active agents
nrworkflow agent active TICKET-1 -w parallel-test
```

### Workflow Management

```bash
nrworkflow workflows                                    # List workflows
nrworkflow init <ticket> -w <workflow>                  # Initialize workflow (auto-creates ticket if needed)
nrworkflow status <ticket> [-w <name>]                  # Human-readable status
nrworkflow progress <ticket> [-w <name>] [--json]       # Live progress
nrworkflow get <ticket> [-w <name>] [field]             # Get state/field
nrworkflow set <ticket> -w <name> <key> <value>         # Set field (requires -w)
```

Note: `-w` is required when multiple workflows exist on a ticket. For `set` and all
state-modifying commands, `-w` is always required.

### Phase Management

```bash
nrworkflow phase start <ticket> <phase> -w <name>
nrworkflow phase complete <ticket> <phase> pass|fail|skipped -w <name>
nrworkflow phase ready <ticket> <phase> -w <name>  # Check if all parallel agents done
```

### Findings

```bash
# Add findings (two syntax modes)
nrworkflow findings add <ticket> <agent-type> <key> '<value>' -w <name>           # legacy: single key-value
nrworkflow findings add <ticket> <agent-type> key:'value' [key2:'value2'] -w <name>  # new: multiple key:value pairs

# Get findings
nrworkflow findings get <ticket> <agent-type> -w <name> [-k key] [--model=cli:model]

# Examples (single agent)
nrworkflow findings add PROJ-1 setup-analyzer files_to_modify '["src/main.py"]' -w feature
nrworkflow findings add PROJ-1 setup-analyzer summary:'Done' status:'passed' -w feature  # multiple at once

# Get specific key(s) - avoids fetching all data
nrworkflow findings get PROJ-1 setup-analyzer -w feature -k summary
nrworkflow findings get PROJ-1 setup-analyzer -w feature -k summary -k status  # multiple keys

# Examples (parallel agents - findings keyed by model)
nrworkflow findings add PROJ-1 setup-analyzer files_to_modify '["src/main.py"]' -w feature --model=claude:sonnet
nrworkflow findings add PROJ-1 setup-analyzer files_to_modify '["src/api.py"]' -w feature --model=opencode:opus
nrworkflow findings get PROJ-1 setup-analyzer -w feature --model=claude:sonnet  # specific model
nrworkflow findings get PROJ-1 setup-analyzer -w feature  # all models grouped: {"claude:sonnet":{...},"opencode:opus":{...}}
nrworkflow findings get PROJ-1 setup-analyzer -w feature -k files_to_modify  # specific key from all: {"claude:sonnet":[...],"opencode:opus":[...]}

# Workflow-level findings (global data shared across agents)
nrworkflow findings add PROJ-1 workflow selected_architecture 'microservices' -w feature
nrworkflow findings add PROJ-1 workflow db_choice:'postgresql' cache:'redis' -w feature
nrworkflow findings get PROJ-1 workflow -w feature                    # all workflow findings
nrworkflow findings get PROJ-1 workflow -w feature -k db_choice       # specific key
```

### Ticket Management

```bash
# Create
nrworkflow ticket create --type=feature --title="..." -d "..."
nrworkflow ticket create --type=feature --title="..." -d "placeholder" --draft

# Initialize workflow
nrworkflow init <ticket> -w feature

# Update
nrworkflow ticket update <ticket> --status=in_progress
nrworkflow ticket close <ticket> --reason="..."

# Dependencies
nrworkflow ticket dep add <child> <parent>

# View
nrworkflow ticket ready                   # Unblocked tickets
nrworkflow ticket status [--json]         # Dashboard
nrworkflow ticket show <ticket> [--json]
nrworkflow ticket list
nrworkflow ticket search <query>
```

## Configuration

### Project Config (`.claude/nrworkflow/config.json`)

All configuration is project-local. There is no global config - each project must have its own config file.

```json
{
  "version": 3,
  "project": "myproject",
  "cli": {
    "default": "claude"
  },
  "spawner": {
    "completion_grace_sec": 60,
    "stats_flush_interval_ms": 2000,
    "stats_flush_max_events": 25,
    "timeout_grace_sec": 5
  },
  "agents": {
    "setup-analyzer": {"model": "sonnet", "max_turns": 50, "timeout": 15},
    "implementor": {"model": "opus", "max_turns": 80, "timeout": 30},
    "test-writer": {"model": "opus", "max_turns": 50, "timeout": 20},
    "qa-verifier": {"model": "opus", "max_turns": 50, "timeout": 20},
    "doc-updater": {"model": "sonnet", "max_turns": 30, "timeout": 10}
  },
  "workflows": {
    "feature": {
      "description": "Full TDD workflow",
      "phases": [
        "setup-analyzer",
        {"agent": "test-writer", "skip_for": ["docs", "simple"]},
        "implementor",
        {"agent": "qa-verifier", "skip_for": ["docs"]},
        "doc-updater"
      ]
    }
  },
  "findings_schema": {
    "setup-analyzer": ["summary", "acceptance_criteria", "files_to_modify"],
    "implementor": ["files_created", "files_modified", "build_result"]
  }
}
```

### Simplified Phase Format

Phases use the agent name as the identifier. Two formats are supported:
- **Simple**: Just the agent name as a string (e.g., `"setup-analyzer"`)
- **With options**: Object with `agent` required field (e.g., `{"agent": "test-writer", "skip_for": ["docs"]}`)

```json
"phases": [
  "setup-analyzer",                                      // Simple: just agent name
  {"agent": "test-writer", "skip_for": ["docs"]},       // With skip rules
  {"agent": "implementor", "parallel": {...}},          // With parallel config
  "doc-updater"                                          // Simple again
]
```

### Spawner Configuration

| Option | Default | Description |
|--------|---------|-------------|
| `completion_grace_sec` | 60 | Wait time for explicit `agent complete` after exit 0 |
| `stats_flush_interval_ms` | 2000 | Interval between stats DB writes (rate limiting) |
| `stats_flush_max_events` | 25 | Max events before forced stats flush |
| `timeout_grace_sec` | 5 | Grace period between SIGTERM and SIGKILL on timeout |

**Completion Semantics:**
- Exit code 0 + explicit `agent complete` within grace period = PASS
- Exit code 0 + no explicit completion = FAIL (reason: `no_complete`)
- Non-zero exit code = FAIL (reason: `exit_code`)
- Timeout = FAIL (reason: `timeout`)

### Parallel Agent Configuration

Phases can be configured to run multiple agents in parallel:

```json
{
  "phases": [
    {
      "id": "investigation",
      "agent": "setup-analyzer",
      "parallel": {
        "enabled": true,
        "models": [
          "claude:opus",
          "claude:sonnet",
          "opencode:anthropic/claude-opus-4-5",
          "opencode:gpt_high",
          "opencode:gpt_max",
          "codex:gpt_high",
          "codex:gpt_xhigh"
        ]
      }
    }
  ]
}
```

The `cli:model` format tells the spawner which CLI adapter to use:
- `claude:opus` → Claude CLI with model "opus"
- `opencode:anthropic/claude-opus-4-5` → Opencode CLI with full model path
- `opencode:opus` → Opencode CLI, auto-mapped to "anthropic/claude-opus-4-5"
- `opencode:gpt_high` → Opencode CLI with gpt-5.2-codex and `--variant high`
- `opencode:gpt_max` → Opencode CLI with gpt-5.2-codex and `--variant max`
- `codex:gpt_high` → Codex CLI with gpt-5.2-codex and reasoning effort "high"
- `codex:gpt_xhigh` → Codex CLI with gpt-5.2-codex and reasoning effort "xhigh"

### Project Discovery

The config file is searched upward from the current directory:

```bash
# Create the config in your project root
mkdir -p /path/to/project/.claude/nrworkflow
cat > /path/to/project/.claude/nrworkflow/config.json << 'EOF'
{
  "project": "myproject",
  "cli": {"default": "claude"},
  "agents": {...},
  "workflows": {...}
}
EOF

# Commands run from anywhere inside the project will find the config
cd /path/to/project/src/components
nrworkflow ticket list  # Uses "myproject" from config.json
```

**Error Handling:**
- If project config file is missing: Error with path where to create it
- If project config is invalid JSON: Error with parse details

## State Storage

State is stored in ticket's `agents_state` field using v4 format.
Multiple workflows can exist per ticket, each with independent state:

```json
{
  "feature": {
    "version": 4,
    "current_phase": "implementation",
    "category": "full",
    "phases": {
      "investigation": {"status": "completed", "result": "pass"},
      "implementation": {"status": "in_progress", "result": null}
    },
    "active_agents": {
      "implementor:claude:opus": {
        "agent_id": "spawn-abc123",
        "agent_type": "implementor",
        "model_id": "claude:opus",
        "cli": "claude",
        "model": "opus",
        "pid": 12345,
        "session_id": "688fa0a0-...",
        "result": null
      },
      "implementor:opencode:opus": {
        "agent_id": "spawn-def456",
        "agent_type": "implementor",
        "model_id": "opencode:opus",
        "cli": "opencode",
        "model": "opus",
        "pid": 12346,
        "session_id": "789ab012-...",
        "result": null
      }
    },
    "findings": {
      "workflow": {                                  ← Workflow-level findings (global)
        "selected_architecture": "microservices",
        "db_choice": "postgresql"
      },
      "setup-analyzer": {                            ← Agent findings (keyed by agent type)
        "claude:sonnet": {"files_to_modify": [...], "patterns": [...]},
        "opencode:opus": {"files_to_modify": [...], "patterns": [...]}
      }
    }
  }
}
```

### Migration

nrworkflow automatically migrates older state formats:
- v2 -> v3: `{"nrworkflow": {...}}` becomes `{"feature": {...}}`
- v3 -> v4: `active_agent` becomes `active_agents` dict keyed by `agent_type:model_id`

The migration happens transparently when state is accessed.

## Guidelines

Located in `~/projects/2026/nrworkflow/guidelines/`:

### findings-schema.md
Standard findings format for all phases:
- Investigation: summary, acceptance_criteria, files_to_modify, patterns
- Test-design: test_files, test_cases, coverage_plan
- Implementation: files_created, files_modified, build_result, test_result
- Verification: verdict, criteria_status, issues
- Docs: docs_updated, summary

### agent-protocol.md
Agent conventions:
- Ticket header format (`## Agent: <type>`, `## Ticket: <id>`)
- Session markers (`${PARENT_SESSION}`, `${CHILD_SESSION}`)
- Completion commands (`nrworkflow agent complete/fail <ticket> <agent-type>`)

## Skill Integration

Skills invoke the spawner directly, passing their session ID and workflow:

```bash
# In /impl skill (SESSION_MARKER is the skill's session UUID)
# Project is auto-discovered from .claude/nrworkflow/config.json
WORKFLOW=feature
nrworkflow agent spawn setup-analyzer $TICKET_ID --session=$SESSION_MARKER -w $WORKFLOW
nrworkflow agent spawn test-writer $TICKET_ID --session=$SESSION_MARKER -w $WORKFLOW    # Auto-skipped if simple/docs
nrworkflow agent spawn implementor $TICKET_ID --session=$SESSION_MARKER -w $WORKFLOW
nrworkflow agent spawn qa-verifier $TICKET_ID --session=$SESSION_MARKER -w $WORKFLOW    # Auto-skipped if docs
nrworkflow agent spawn doc-updater $TICKET_ID --session=$SESSION_MARKER -w $WORKFLOW
```

## Debugging

```bash
# Check agent configuration
nrworkflow agent list

# Preview prompt without spawning
nrworkflow agent preview implementor PROJ-123 -w feature

# Check ticket state (use -w when multiple workflows exist)
nrworkflow status PROJ-123 -w feature
nrworkflow progress PROJ-123 -w feature --json
nrworkflow get PROJ-123 -w feature

# Check findings
nrworkflow findings get PROJ-123 setup-analyzer -w feature

# Kill stuck agent
nrworkflow agent kill PROJ-123 -w feature
```

## Server Architecture

nrworkflow uses a client-server architecture where the CLI communicates with the server via Unix socket:

```
User runs: nrworkflow ticket list
              │
              ▼
        ┌───────────┐
        │    CLI    │  (thin client - no DB access)
        └─────┬─────┘
              │ connect + JSON request
              ▼
        /tmp/nrworkflow/nrworkflow.sock
              │
              ▼
        ┌─────────────────────────────────┐
        │       nrworkflow serve          │
        │  ┌─────────────────────────┐    │
        │  │    Service Layer        │    │
        │  │  (business logic)       │    │
        │  └───────────┬─────────────┘    │
        │              │                  │
        │  ┌───────────▼─────────────┐    │
        │  │    DB Connection Pool   │    │
        │  │    (10 max, 5 idle)     │    │
        │  └─────────────────────────┘    │
        │                                 │
        │  Also: HTTP :6587 (for web UI)  │
        └─────────────────────────────────┘
```

**Socket Protocol (JSON-RPC style):**
```json
// Request
{"id": "uuid", "method": "ticket.list", "params": {"status": "open"}, "project": "myproject"}

// Response
{"id": "uuid", "result": [{"id": "T-1", "title": "..."}], "error": null}
```

**Socket Location:**
- Default: `/tmp/nrworkflow/nrworkflow.sock`
- Override: `NRWORKFLOW_SOCKET` env var
- Permissions: `0600` (owner only)

## Web UI

A web interface is available for managing tickets visually:

```bash
# Quick start - restart both servers (kills existing, rebuilds, starts in background)
./restart.sh

# Or start manually:
# 1. Start the server (serves both HTTP API and Unix socket)
nrworkflow serve

# 2. Start the UI (in another terminal)
cd ui && npm run dev

# 3. Open http://localhost:5173

# Stop servers
./stop.sh
```

### Server Scripts

| Script | Purpose |
|--------|---------|
| `restart.sh` | Kill existing servers, rebuild BE + UI, start both in background |
| `stop.sh` | Stop running BE + UI servers |
| `ui/start-server.sh` | Start both servers in foreground (interactive mode) |

Logs are written to `logs/backend.log` and `logs/ui.log` when using `restart.sh`.

Features:
- Dashboard with ticket counts and status overview
- Ticket list with filtering and search
- Ticket detail view with workflow timeline
- Agents page showing recent agents across all projects with live polling
- Live tracking with real-time agent stdout messages
- Findings display with workflow-level and agent findings separated
- Create/edit/close tickets
- Multi-project support via project selector
- Settings page for project management (create/update/delete)

### Live Tracking

When "Live" toggle is enabled on a ticket's workflow tab, the UI polls for:
1. Ticket state updates (workflow progress, phase status)
2. Agent session messages via `/api/v1/tickets/{id}/agents`

Agent sessions display:
- Current status (running, completed, failed, timeout)
- Model ID (e.g., "claude:sonnet")
- Last 50 messages (newest first) with detailed tool info (truncated to 200 chars each)
- Message stats (tool/skill/text counts) via hover tooltip

### Message Format

The spawner parses JSON stream output and formats messages with tool details:

```
[Bash] git status
[Read] /path/to/file.ts
[Grep] pattern in src/api/
[Skill] skill:jira-ticket REF-12425
[Task] codebase-explorer: Find error handlers
[WebFetch] https://api.example.com/docs
[Edit] /src/main.ts
[stderr] API rate limit warning
```

Both Claude CLI (`stream-json`) and Opencode (`json`) output formats are supported with automatic normalization:
- **Tool names**: Normalized to title case (`bash` → `Bash`)
- **Input location**: Claude uses `part.input`, Opencode uses `part.state.input`
- **Field names**: Both snake_case (`file_path`) and camelCase (`filePath`) supported
- **Long messages**: Truncated as `START (300 chars) ... [N chars] ... END (150 chars)`
- **Stderr**: Captured and displayed with `[stderr]` prefix for debugging
- **Scanner buffer**: 10MB limit for large JSON outputs (file reads, diffs)

### API Endpoints

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
PUT /api/v1/tickets/:id
DELETE /api/v1/tickets/:id

# Workflow state
GET /api/v1/tickets/:id/workflow
PUT /api/v1/tickets/:id/workflow

# Agent sessions
GET /api/v1/tickets/:id/agents
GET /api/v1/tickets/:id/agents?phase=investigation

# Recent agents (cross-project, no X-Project header required)
GET /api/v1/agents/recent
GET /api/v1/agents/recent?limit=10

# Response example:
{
  "ticket_id": "TICKET-123",
  "sessions": [
    {
      "id": "uuid",
      "ticket_id": "TICKET-123",
      "phase": "investigation",
      "workflow": "feature",
      "agent_type": "setup-analyzer",
      "model_id": "claude:sonnet",
      "status": "running",
      "last_messages": ["Analyzing codebase...", "[Read]", "[Grep]"],
      "message_stats": {"tool:Read": 15, "tool:Grep": 8, "text": 5},
      "created_at": "2026-02-02T10:00:00Z",
      "updated_at": "2026-02-02T10:05:00Z"
    }
  ],
  "findings": {
    "setup-analyzer": {
      "category": "full",
      "patterns": ["REST API", "Service layer"],
      "acceptance_criteria": ["User can login", "Session persists"]
    }
  }
}
```

Server configuration in `~/projects/2026/nrworkflow/config.json`:
```json
{
  "server": {
    "port": 6587,
    "cors_origins": ["http://localhost:5173"]
  }
}
```

See [ui/README.md](ui/README.md) for full documentation.

## Files

| File | Purpose |
|------|---------|
| `nrworkflow/` | Go CLI source code |
| `nrworkflow.data` | SQLite database (tickets, projects, sessions) |
| `guidelines/*.md` | Protocols (findings-schema, agent-protocol) |
| `ui/` | React web UI for ticket management |
| `restart.sh` | Rebuild and restart BE + UI servers (background) |
| `stop.sh` | Stop running servers |

**Project files (`.claude/nrworkflow/`):**
| File | Purpose |
|------|---------|
| `config.json` | Project config (workflows, agents, settings) |
| `agents/*.md` | Agent templates |

## Building from Source

```bash
cd ~/projects/2026/nrworkflow/nrworkflow

# Build
make build

# Install to /usr/local/bin
sudo make install

# Or just copy
sudo cp nrworkflow /usr/local/bin/
```

Requirements:
- Go 1.21+
- No CGO required (pure Go SQLite via modernc.org/sqlite)
# nrworkflow
