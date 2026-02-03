# Claude Code Instructions for nrworkflow

## Overview

This is the nrworkflow system - a multi-workflow state management tool for ticket implementation with spawned AI agents.

**Main documentation**: [README.md](README.md)

## Mandatory Rules

### 1. Update Documentation on Any Change

When making changes, you MUST update all affected documentation:

#### 1a. WORKFLOW.md (System Diagrams)

Update [WORKFLOW.md](WORKFLOW.md) when modifying:
- `nrworkflow/` - Go CLI commands, state structure
- `nrworkflow/internal/spawner/` - Spawner flow, session handling
- `config.json` - Workflows, phases, agents
- `agents/*.base.md` - Agent templates
- `guidelines/*.md` - Protocols, schemas
- `EXAMPLE_*.md` - Skill examples

**What to update:**
- Add/remove boxes in diagrams if commands or phases change
- Update state machine diagrams if statuses change
- Update file structure section if files added/removed
- Update data flow if new data is stored/read
- Update session tracking if session handling changes
- Add timestamp at bottom: `*Last updated: YYYY-MM-DD*`

#### 1b. README.md (User Documentation)

Update [README.md](README.md) when:
- Adding/removing/changing CLI commands → update Commands section
- Changing command arguments or flags → update usage examples
- Adding new features → add to relevant section
- Changing configuration options → update Configuration section
- Adding new files → update Architecture section

#### 1c. Guidelines (Protocols & Schemas)

Update relevant guidelines when:
- Changing agent behavior → `guidelines/agent-protocol.md`
- Changing findings format → `guidelines/findings-schema.md`
- Adding new phase with findings → add schema to `findings-schema.md`

#### 1d. Skill Examples

Update skill examples when:
- Changing spawn command syntax → `EXAMPLE_IMPL_SKILL.md`
- Changing ticket creation → `EXAMPLE_PREP_SKILL.md`
- Adding new workflow phases → both skill examples

#### 1e. UI (Web Interface)

Update [ui/](ui/) when:
- Changing API endpoints → `ui/src/api/`
- Changing data models → `ui/src/types/`
- Adding new features → relevant components

See [ui/CLAUDE.md](ui/CLAUDE.md) for UI-specific instructions.

### 2. Session Parameter is Mandatory

The `--session` parameter is **required** for `agent spawn`:

```bash
nrworkflow agent spawn <type> <ticket> --session=<parent_uuid> -w <workflow>
```

This links spawned agents to the orchestrating session. Never remove this requirement.

**Note:** Project is auto-discovered from `.claude/nrworkflow/config.json` (searched upward from cwd) or `NRWORKFLOW_PROJECT` env var.

### 3. Phase Sequence is Enforced

Agents can only be spawned in workflow phase order. The spawner validates:
- Current phase matches expected next phase
- Prior phases are completed or skipped
- Category-based skip rules are applied

### 4. State is Stored in Ticket

All nrworkflow state is stored in the ticket's `agents_state` field as JSON. The state includes:
- Workflow and phase information
- Active agent tracking (pid, session_id)
- Findings from each phase
- Agent history

### 5. Findings Schema Must Be Followed

When agents store findings, they must follow the schema in `guidelines/findings-schema.md`. Required fields vary by phase.

## Key Files

| File | Purpose |
|------|---------|
| `nrworkflow/` | Go CLI source code |
| `nrworkflow/internal/cli/` | Command handlers (thin socket clients) |
| `nrworkflow/internal/client/` | Unix socket client |
| `nrworkflow/internal/socket/` | Unix socket server |
| `nrworkflow/internal/service/` | Business logic layer |
| `nrworkflow/internal/spawner/` | Agent spawner |
| `nrworkflow/internal/db/` | SQLite database + connection pool |
| `nrworkflow/internal/types/` | Shared request/response types |
| `config.json` | Workflows, agents, timeouts |
| `WORKFLOW.md` | **System diagrams - KEEP UPDATED** |
| `README.md` | User documentation |
| `guidelines/agent-protocol.md` | Agent conventions |
| `guidelines/findings-schema.md` | Findings format |
| `ui/` | React web interface |
| `ui/CLAUDE.md` | UI maintenance instructions |

## Architecture Principles

1. **Client-Server**: CLI communicates with server via Unix socket
2. **Server Required**: `nrworkflow serve` must be running for CLI to work
3. **Go binary**: Single `nrworkflow` binary serves both client and server roles
4. **Project-scoped**: Project discovered from `.claude/nrworkflow/config.json` or `NRWORKFLOW_PROJECT` env
5. **Single database**: `~/projects/2026/nrworkflow/nrworkflow.data` (SQLite, global for all projects)
6. **Connection Pool**: DB uses connection pooling (10 max, 5 idle)
7. **Service Layer**: Business logic separated from CLI and socket handlers
8. **Spawner-driven**: State management happens in the spawner (direct, not via socket)
9. **Session linking**: Parent session passed explicitly via CLI
10. **Phase validation**: Can't spawn out of order
11. **Category-based skipping**: `skip_for` rules in workflow config

## Common Tasks

### Adding a New Agent Type

1. Create `agents/<type>.base.md` template
2. Add to workflow phases in `config.json`
3. Add agent config (model, max_turns, timeout) to `config.json`
4. **Documentation updates:**
   - `guidelines/findings-schema.md` - add findings schema for new phase
   - `WORKFLOW.md` - add agent to diagrams, update file structure
   - `README.md` - add to Base Agent Templates table

### Adding a New Workflow

1. Add workflow definition to `config.json`
2. Ensure all referenced agents exist
3. **Documentation updates:**
   - `WORKFLOW.md` - add workflow to diagrams
   - `README.md` - add to Workflows table

### Modifying State Structure

1. Update state initialization in `nrworkflow/internal/service/workflow.go`
2. Update any code reading that state
3. **Documentation updates:**
   - `WORKFLOW.md` - update state diagrams and data flow
   - `README.md` - update State Storage section if user-visible

### Changing CLI Commands

1. Update CLI command (thin client) in `nrworkflow/internal/cli/`
2. Update socket handler in `nrworkflow/internal/socket/handler.go`
3. Update service in `nrworkflow/internal/service/`
4. Rebuild: `cd nrworkflow && make build`
5. **Documentation updates:**
   - `WORKFLOW.md` - update diagrams if flow changes
   - `README.md` - update Commands section, usage examples
   - `EXAMPLE_IMPL_SKILL.md` / `EXAMPLE_PREP_SKILL.md` - if spawn/ticket commands change
   - `guidelines/agent-protocol.md` - if agent-facing commands change

### Adding New Socket Methods

1. Add request type in `nrworkflow/internal/types/request.go`
2. Add service method in `nrworkflow/internal/service/`
3. Add handler case in `nrworkflow/internal/socket/handler.go`
4. Add CLI command in `nrworkflow/internal/cli/`

### Adding New Configuration Options

1. Add to `config.json` (global and/or project)
2. Update code to read the new option in `nrworkflow/internal/service/workflow.go`
3. **Documentation updates:**
   - `README.md` - update Configuration section
   - `WORKFLOW.md` - if it affects flow or data

### Modifying API Endpoints (HTTP)

1. Update handlers in `nrworkflow/internal/api/`
2. Update routes in `nrworkflow/internal/api/server.go`
3. Consider if the same logic should be in socket handler
4. **Documentation updates:**
   - `README.md` - update API Endpoints section
   - `ui/src/api/` - update corresponding API client
   - `ui/src/types/` - update TypeScript types if needed

## Web UI

The web interface is in `ui/`. See [ui/README.md](ui/README.md) for setup and [ui/CLAUDE.md](ui/CLAUDE.md) for development instructions.

```bash
# Start API server
nrworkflow serve

# Start UI dev server (in another terminal)
cd ui && npm run dev

# Open http://localhost:5173
```

Key UI files:
- `ui/src/api/client.ts` - API client with X-Project header
- `ui/src/api/projects.ts` - Project API functions
- `ui/src/api/tickets.ts` - Ticket and workflow API functions
- `ui/src/stores/projectStore.ts` - Project selection state
- `ui/src/types/` - TypeScript types matching Go models
