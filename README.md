# nrworkflow

Multi-workflow state management for ticket implementation. Supports multiple workflows per ticket, with reusable agents across workflows. A single ticket can have multiple workflows initialized (e.g., running both `feature` and `bugfix` workflows on the same ticket).

**v4** adds support for parallel agents - running 1-3 agents (Claude, OpenAI, Gemini, etc.) simultaneously during a single phase, each identified by `cli:model` format.

## Documentation

| Document | Purpose |
|----------|---------|
| **README.md** | User documentation (this file) |
| [WORKFLOW.md](WORKFLOW.md) | System diagrams and architecture |
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
~/projects/2026/nrworkflow/             # GLOBAL (NRWORKFLOW_HOME)
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
├── config.json                         # Global defaults (workflows, agents)
├── agents/                             # Base agent templates
│   ├── setup-analyzer.base.md
│   ├── implementor.base.md
│   ├── test-writer.base.md
│   ├── qa-verifier.base.md
│   └── doc-updater.base.md
└── guidelines/                         # Shared protocols
    ├── findings-schema.md
    └── agent-protocol.md

<project>/.claude/nrworkflow/           # PROJECT (required for commands)
├── config.json                         # Project config with "project" field
└── overrides/                          # Agent overrides (optional)
    └── implementor.md                  # Project-specific instructions
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

## Base Agent Templates

Located in `~/projects/2026/nrworkflow/agents/`:

| Template | Purpose | Model |
|----------|---------|-------|
| `setup-analyzer.base.md` | Investigation and context gathering | sonnet |
| `implementor.base.md` | Code implementation | opus |
| `test-writer.base.md` | TDD test design | opus |
| `qa-verifier.base.md` | Verification and quality checks | opus |
| `doc-updater.base.md` | Documentation updates | sonnet |

### Template Variables

Templates use placeholders injected by the spawner:
- `${AGENT}` - Agent type (e.g., "setup-analyzer", "implementor")
- `${TICKET_ID}` - Current ticket ID
- `${PARENT_SESSION}` - Parent session UUID
- `${CHILD_SESSION}` - This agent's session UUID
- `${PROJECT_SPECIFIC}` - Replaced with project override content (or empty)
- `${WORKFLOW}` - Current workflow name (e.g., "feature", "bugfix")
- `${MODEL_ID}` - Full model identifier in cli:model format (e.g., "claude:sonnet")
- `${MODEL}` - Just the model name (e.g., "sonnet")

### Project Overrides

Create project-specific template overrides in `<projectRoot>/.claude/nrworkflow/overrides/<agent>.md`.
These overrides are injected into the `${PROJECT_SPECIFIC}` placeholder in base templates.

**Requirements:**
1. Project must have `root_path` set (via `--root` flag on project create)
2. Override files must be named `<agent-type>.md` (e.g., `implementor.md`, `setup-analyzer.md`)

**Example Setup:**
```bash
# 1. Create project with root path
nrworkflow project create swift-app --name "Swift App" --root /Users/dev/swift-app

# 2. Create overrides directory
mkdir -p /Users/dev/swift-app/.claude/nrworkflow/overrides

# 3. Create agent override
cat > /Users/dev/swift-app/.claude/nrworkflow/overrides/implementor.md << 'EOF'
## Project Context
- Language: Swift 5.9
- Framework: SwiftUI + Combine

## Build & Test Commands
- Build: `swift build`
- Test: `swift test`

## Available Skills
- `/build` - Build the project
- `/test` - Run tests

## Project Conventions
- All new files must have copyright header
- Tests go in Tests/ mirroring Sources/ structure
EOF

# 4. Preview to verify override is loaded (from project directory)
cd /Users/dev/swift-app
nrworkflow agent preview implementor TICKET-1 -w feature
# Output should include "## Project Context" section
```

**Behavior:**
- If override file exists: Its content replaces `${PROJECT_SPECIFIC}` in the template
- If override file is missing: `${PROJECT_SPECIFIC}` is replaced with empty string (normal case)
- Override content is injected as-is (no additional variable expansion)

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
nrworkflow findings add <ticket> <agent-type> <key> '<value>' -w <name> [--model=cli:model]
nrworkflow findings get <ticket> <agent-type> -w <name> [--model=cli:model]

# Examples (single agent)
nrworkflow findings add PROJ-1 setup-analyzer files_to_modify '["src/main.py"]' -w feature
nrworkflow findings add PROJ-1 implementor build_result '"pass"' -w feature

# Examples (parallel agents - findings keyed by model)
nrworkflow findings add PROJ-1 setup-analyzer files_to_modify '["src/main.py"]' -w feature --model=claude:sonnet
nrworkflow findings add PROJ-1 setup-analyzer files_to_modify '["src/api.py"]' -w feature --model=opencode:opus
nrworkflow findings get PROJ-1 setup-analyzer -w feature --model=claude:sonnet  # specific model
nrworkflow findings get PROJ-1 setup-analyzer -w feature  # all models grouped: {"claude:sonnet":{...},"opencode:opus":{...}}
nrworkflow findings get PROJ-1 setup-analyzer files_to_modify -w feature  # specific key from all: {"claude:sonnet":[...],"opencode:opus":[...]}
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

### Global Config (`~/projects/2026/nrworkflow/config.json`)

```json
{
  "version": 3,
  "cli": {
    "default": "claude"
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
        {"id": "investigation", "agent": "setup-analyzer"},
        {"id": "test-design", "agent": "test-writer", "skip_for": ["docs", "simple"]},
        {"id": "implementation", "agent": "implementor"},
        {"id": "verification", "agent": "qa-verifier", "skip_for": ["docs"]},
        {"id": "docs", "agent": "doc-updater"}
      ]
    }
  },
  "findings_schema": {
    "setup-analyzer": ["summary", "acceptance_criteria", "files_to_modify"],
    "implementor": ["files_created", "files_modified", "build_result"]
  }
}
```

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

### Project Config (`.claude/nrworkflow/config.json`)

Project-specific configuration is loaded from `<projectRoot>/.claude/nrworkflow/config.json`. This file is searched upward from the current directory:

```bash
# Create the config in your project root
mkdir -p /path/to/project/.claude/nrworkflow
cat > /path/to/project/.claude/nrworkflow/config.json << 'EOF'
{
  "project": "myproject"
}
EOF

# Commands run from anywhere inside the project will find the config
cd /path/to/project/src/components
nrworkflow ticket list  # Uses "myproject" from config.json
```

Full config example:
```json
{
  "project": "myproject",
  "cli": {
    "default": "opencode"
  },
  "agents": {
    "implementor": {"model": "opus", "max_turns": 100},
    "setup-analyzer": {"timeout": 20}
  },
  "workflows": {
    "custom": {
      "description": "Custom project workflow",
      "phases": [
        {"id": "implementation", "agent": "implementor"}
      ]
    }
  }
}
```

**Merge Behavior:**
- `cli.default`: Project value overrides global if set
- `agents`: Per-agent fields are merged (project values override individual fields like `model`, `max_turns`, `timeout`); new agents defined only in project config are added
- `workflows`: Project workflows completely replace global workflows of the same name; new workflows defined only in project config are added

**Commands Using Merged Config:**
All workflow and agent commands use merged config:
- `nrworkflow workflows` - Lists both global and project-specific workflows
- `nrworkflow init` - Validates workflow exists in merged config
- `nrworkflow agent list` - Lists agents from all workflows (global + project)
- `nrworkflow agent spawn` - Uses merged config for workflow validation and agent settings
- `nrworkflow agent preview` - Uses merged config for template and settings
- `nrworkflow status` - Uses merged config to display workflow phases

**Error Handling:**
- If project config file is missing: Uses global config only (silent, normal case)
- If project config is invalid JSON: Logs warning, uses global config only

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
      "setup-analyzer": {
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

See `EXAMPLE_IMPL_SKILL.md` and `EXAMPLE_PREP_SKILL.md` for complete examples.

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
| `config.json` | Global defaults (workflows, agents, server) |
| `agents/*.base.md` | Base agent templates |
| `guidelines/*.md` | Protocols (findings-schema, agent-protocol) |
| `EXAMPLE_PREP_SKILL.md` | Planning skill example |
| `EXAMPLE_IMPL_SKILL.md` | Implementation skill example |
| `ui/` | React web UI for ticket management |
| `restart.sh` | Rebuild and restart BE + UI servers (background) |
| `stop.sh` | Stop running servers |

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
