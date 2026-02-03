# nrworkflow System Diagram

## High-Level Overview

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

## Unix Socket IPC Architecture

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

## Skill Workflows

### /prep - Planning Skill

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

### /impl - Implementation Skill

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

## Agent Spawner Flow

```
nrworkflow agent spawn <type> <ticket> -p <project> --session=<parent_uuid> -w <workflow>
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        SPAWNER FLOW (Go)                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  1. VALIDATION                                                       │
│     ├── Check workflow is initialized on ticket                     │
│     │   └── Error: "workflow 'X' not initialized. Run: nrworkflow   │
│     │              workflow init <ticket> -w <workflow>"            │
│     ├── Check workflow definition exists in config                  │
│     ├── Check phase sequence (can't spawn out of order)             │
│     └── Check skip_for rules (category-based)                       │
│         │                                                            │
│         ├── If skip: mark phase skipped, return 0                   │
│         │                                                            │
│         ▼                                                            │
│  2. LOAD AGENT CONFIG (from ~/projects/2026/nrworkflow/config.json)               │
│     ├── Get model, max_turns, timeout for agent type                │
│     │   ├── model: from agents[type].model                          │
│     │   ├── max_turns: from agents[type].max_turns                  │
│     │   └── timeout: from agents[type].timeout                      │
│     │                                                                │
│         ▼                                                            │
│  3. START PHASE & SPAWN AGENT                                        │
│     ├── Update ticket state: phase = in_progress                    │
│     ├── Generate agent_id: spawn-<uuid8>                            │
│     ├── Generate child_session_id: <uuid>                           │
│     ├── Load and expand template with variables                     │
│     ├── Write prompt to temp file                                   │
│     ├── Spawn CLI process (claude/opencode)                         │
│     └── Register agent start in ticket state                        │
│         │                                                            │
│         ▼                                                            │
│  4. MONITOR (goroutines)                                             │
│     ├── Stdout goroutine: parse JSON stream, print status           │
│     │   └── Track message stats (text, tool:*, skill:*, result:*)   │
│     ├── Stderr goroutine: print errors                              │
│     ├── Wait for process completion                                 │
│     └── Handle timeout (kill process)                               │
│         │                                                            │
│         ▼                                                            │
│  5. FINALIZE                                                         │
│     ├── Determine result from exit code                             │
│     ├── Save message stats to agent_sessions table                  │
│     ├── Register agent stop in ticket state                         │
│     ├── Move agent to history                                       │
│     └── Update phase status (completed/failed)                      │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Session Tracking

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

## Parallel Agent Flow

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

## Ticket State Machine

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

## Phase State Machine

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

## File Structure

```
~/projects/2026/nrworkflow/                          # GLOBAL INSTALLATION
├── nrworkflow/                         # Go CLI source
│   ├── cmd/nrworkflow/main.go          # Entry point
│   ├── internal/
│   │   ├── cli/                        # Command handlers (thin socket clients)
│   │   │   ├── root.go                 # Root cmd, global flags, GetClient()
│   │   │   ├── project.go              # project create/list/show/delete
│   │   │   ├── ticket.go               # ticket create/update/close/...
│   │   │   ├── workflow.go             # workflows, init, status, get, set
│   │   │   ├── agent.go                # agent spawn/preview/list/...
│   │   │   ├── findings.go             # findings add/get
│   │   │   └── serve.go                # Server startup (socket + HTTP)
│   │   ├── client/                     # Unix socket client
│   │   │   ├── client.go               # Socket connection & execute
│   │   │   └── output.go               # Response formatting
│   │   ├── socket/                     # Unix socket server
│   │   │   ├── server.go               # Listener, accept loop, cleanup
│   │   │   ├── handler.go              # Request dispatch to services
│   │   │   └── protocol.go             # Request/Response/Error types
│   │   ├── service/                    # Business logic layer
│   │   │   ├── ticket.go               # Ticket operations
│   │   │   ├── project.go              # Project operations
│   │   │   ├── workflow.go             # Workflow operations
│   │   │   ├── agent.go                # Agent operations
│   │   │   └── findings.go             # Findings operations
│   │   ├── spawner/
│   │   │   ├── spawner.go              # Agent spawner (parallel support)
│   │   │   └── cli_adapter.go          # CLI adapters (claude, opencode)
│   │   ├── db/
│   │   │   ├── db.go                   # SQLite database
│   │   │   └── pool.go                 # Connection pooling
│   │   ├── types/                      # Shared request/response types
│   │   │   └── request.go              # Request structs
│   │   ├── model/                      # Data models
│   │   ├── repo/                       # Repositories
│   │   └── api/                        # HTTP API handlers (web UI)
│   ├── go.mod
│   └── Makefile
├── nrworkflow.data                     # SQLite database (fixed location)
├── config.json                         # Global defaults (workflows, agents)
├── agents/                             # Base agent templates
│   ├── setup-analyzer.base.md          # Investigation agent
│   ├── test-writer.base.md             # TDD test design agent
│   ├── implementor.base.md             # Implementation agent
│   ├── qa-verifier.base.md             # Verification agent
│   └── doc-updater.base.md             # Documentation agent
├── guidelines/
│   ├── findings-schema.md              # Findings format spec
│   └── agent-protocol.md               # Agent conventions
├── WORKFLOW.md                         # This diagram
├── CLAUDE.md                           # Maintenance rules
└── README.md                           # Main documentation

<project>/.claude/nrworkflow/           # PROJECT-SPECIFIC (optional)
├── config.json                         # Project config (merged with global)
└── overrides/                          # Agent template overrides
    └── <agent>.md                      # Project-specific instructions
```

## Database Schema

```
┌─────────────────────────────────────────────────────────────────────┐
│                     DATABASE TABLES                                  │
│              (~/projects/2026/nrworkflow/nrworkflow.data)                       │
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
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│                                                                      │
│  TICKETS_FTS (Full-text search)                                      │
│    project_id, id, title, description                                │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Data Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                         DATA FLOW                                    │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  /prep                                                               │
│    │                                                                 │
│    └──► Ticket (nrworkflow.data)                                    │
│          ├── title, type, priority, status                          │
│          ├── description (## Vision, ## Acceptance Criteria)        │
│          └── agents_state: { <workflow>: {...}, ... }   (v4 format) │
│                                                                      │
│  /impl                                                               │
│    │                                                                 │
│    ├──► nrworkflow state (in agents_state, per workflow)            │
│    │     ├── version: 4                                             │
│    │     ├── category, current_phase                                │
│    │     ├── phases: { <phase>: {status, result} }                  │
│    │     ├── active_agents: {<key>: {id, type, model_id, pid...}}   │
│    │     ├── agent_history: [{id, type, duration, result}]          │
│    │     └── findings: { <agent>: {key: value} }                    │
│    │                                                                 │
│    └──► agent_sessions (nrworkflow.data, separate table)            │
│          ├── Real-time session tracking                             │
│          ├── last_messages: last 50 messages, newest first (200 char)
│          └── message_stats: tool/skill/text counts (saved on done)  │
│                                                                      │
│  Agent                                                               │
│    │                                                                 │
│    ├──► Reads: nrworkflow findings get <ticket> <agent> -p <proj> -w│
│    └──► Writes: nrworkflow findings add <ticket> <agent> <k> <v> -p │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Config Merging Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                    CONFIG LOADING & MERGING                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Commands using merged config:                                       │
│    - workflows        (list all workflows)                           │
│    - init             (validate workflow exists)                     │
│    - status           (display workflow phases)                      │
│    - agent list       (list agents from all workflows)               │
│    - agent spawn      (workflow validation + agent settings)         │
│    - agent preview    (template + settings)                          │
│       │                                                              │
│       ▼                                                              │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ 1. Get Project Root Path                                     │    │
│  │    ├── From PersistentPreRunE: search upward for             │    │
│  │    │   .claude/nrworkflow/config.json (sets ProjectRoot)     │    │
│  │    ├── Or from database: project.root_path                   │    │
│  │    └── Fall back to "." if not set                           │    │
│  └──────────────────────────┬──────────────────────────────────┘    │
│                             │                                        │
│                             ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ 2. Load Global Config                                        │    │
│  │    └── ~/projects/2026/nrworkflow/config.json                             │    │
│  └──────────────────────────┬──────────────────────────────────┘    │
│                             │                                        │
│                             ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ 3. Load Project Config (if root_path set)                    │    │
│  │    └── <root_path>/.claude/nrworkflow/config.json           │    │
│  │        ├── Missing: use global only                          │    │
│  │        └── Invalid JSON: warn + use global only              │    │
│  └──────────────────────────┬──────────────────────────────────┘    │
│                             │                                        │
│                             ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ 4. Merge Configs                                             │    │
│  │    ├── cli.default: project overrides global                │    │
│  │    ├── agents: per-agent field merge                         │    │
│  │    │   ├── project model/max_turns/timeout override global   │    │
│  │    │   └── new agents in project config are added            │    │
│  │    └── workflows:                                            │    │
│  │        ├── project replaces global (same name)               │    │
│  │        └── new workflows in project config are added         │    │
│  └──────────────────────────┬──────────────────────────────────┘    │
│                             │                                        │
│                             ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │ 5. Load Template                                             │    │
│  │    ├── Base: ~/projects/2026/nrworkflow/agents/<agent>.base.md            │    │
│  │    └── Override: <root_path>/.claude/nrworkflow/overrides/   │    │
│  │                  <agent>.md → ${PROJECT_SPECIFIC}             │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## CLI Command Reference

```
┌─────────────────────────────────────────────────────────────────────┐
│                     CLI COMMANDS (Go binary)                         │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  IMPORTANT: Server must be running for CLI commands to work!        │
│    nrworkflow serve    # Start before using other commands          │
│                                                                      │
│  Global flags:                                                       │
│    -p, --project    Project ID (or NRWORKFLOW_PROJECT env)          │
│    -D, --data       Database path (default: project root)           │
│                                                                      │
│  PROJECT COMMANDS                                                    │
│    project create <id> --name "Name"                                │
│    project list                                                      │
│    project show <id>                                                 │
│    project delete <id>                                               │
│                                                                      │
│  TICKET COMMANDS (require -p)                                        │
│    ticket create --title "..." --type feature -d "..."              │
│    ticket list [--status open] [--type bug]                         │
│    ticket show <id> [--json]                                         │
│    ticket update <id> [--status ...] [--title ...]                  │
│    ticket close <id> [--reason "..."]                               │
│    ticket delete <id> [--force]                                      │
│    ticket search <query>                                             │
│    ticket dep add <child> <parent>                                   │
│    ticket ready                                                      │
│    ticket status [--json]                                            │
│                                                                      │
│  WORKFLOW COMMANDS (require -p)                                      │
│    workflows                   # List all workflows                  │
│    init <ticket> -w <workflow>  # Initialize (auto-creates ticket)  │
│    status <ticket> [-w name]   # Human-readable status              │
│    progress <ticket> [-w name] [--json]                              │
│    get <ticket> [-w name] [field]                                    │
│    set <ticket> -w <name> <key> <value>                              │
│                                                                      │
│  PHASE COMMANDS (require -p and -w)                                  │
│    phase start <ticket> <phase>                                      │
│    phase complete <ticket> <phase> pass|fail|skipped                │
│    phase ready <ticket> <phase>                                      │
│                                                                      │
│  AGENT COMMANDS                                                      │
│    agent list                  # Available agent types               │
│    agent spawn <type> <ticket> -p <proj> --session=<uuid> -w <wf>   │
│        NOTE: spawn runs directly (not via socket)                   │
│    agent preview <type> <ticket> -p <proj> [-w name]                │
│        NOTE: preview runs directly (not via socket)                 │
│    agent active <ticket> -p <proj> -w <name>                         │
│    agent start <ticket> <id> <type> -p <proj> -w <name> [--pid ...]  │
│    agent stop <ticket> <id> <result> -p <proj> -w <name>             │
│    agent complete <ticket> <type> -p <proj> -w <name>                │
│    agent fail <ticket> <type> -p <proj> -w <name>                    │
│    agent kill <ticket> -p <proj> -w <name> [--model ...]             │
│    agent retry <ticket> -p <proj> -w <name>                          │
│                                                                      │
│  FINDINGS COMMANDS (require -p and -w)                               │
│    findings add <ticket> <agent> <key> <value>                       │
│    findings get <ticket> <agent> [key]                               │
│                                                                      │
│  SERVER COMMANDS                                                     │
│    serve [--port 6587]         # Start server (socket + HTTP)       │
│    init-db                     # Initialize database                 │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘

Socket Method Mapping:
┌────────────────────┬──────────────────┬───────────┐
│ CLI Command        │ Socket Method    │ Streaming │
├────────────────────┼──────────────────┼───────────┤
│ ticket create      │ ticket.create    │ No        │
│ ticket list        │ ticket.list      │ No        │
│ ticket show        │ ticket.get       │ No        │
│ ticket update      │ ticket.update    │ No        │
│ ticket close       │ ticket.close     │ No        │
│ ticket delete      │ ticket.delete    │ No        │
│ ticket search      │ ticket.search    │ No        │
│ project create     │ project.create   │ No        │
│ project list       │ project.list     │ No        │
│ workflow init      │ workflow.init    │ No        │
│ phase start        │ phase.start      │ No        │
│ phase complete     │ phase.complete   │ No        │
│ findings add       │ findings.add     │ No        │
│ agent spawn        │ (direct)         │ N/A       │
│ agent preview      │ (direct)         │ N/A       │
└────────────────────┴──────────────────┴───────────┘
```

---

*Last updated: 2026-02-03 (opencode adapter supports --variant for GPT reasoning effort)*
