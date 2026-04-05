# nrflow

A self-hosted control plane for AI engineering workflows. nrflow orchestrates coding agents across layered workflows, isolated git worktrees, and structured findings handoffs, with real-time monitoring and browser takeover when automation needs human control.

## Features

- **Vendor-agnostic orchestration** — run workflows across Claude CLI, Opencode, and Codex
- **Layered execution model** — fan out parallel agents in the same layer, then fan in to the next validated stage
- **Structured findings handoff** — agents write findings that later agents can pull directly into prompts
- **Ticket and project workflows** — run isolated ticket-scoped worktree jobs or project-scoped workflows without a ticket
- **Human takeover when needed** — start interactively, enter plan mode, take control of a running agent, or resume a finished session from the browser
- **Recovery built in** — low-context continuation, stall restart, manual restart, and retry from the failed layer
- **Verifier callbacks** — later stages can re-run earlier layers with explicit callback instructions
- **Sequential ticket chains** — execute dependency-aware ticket sequences with crash recovery
- **Automatic merge handling** — merge back from worktrees, invoke a conflict-resolver agent on failure, and optionally push after merge
- **Real-time visibility** — WebSocket-driven workflow graphs, logs, findings, final results, and error tracking
- **Prompt and model controls** — template variables, findings expansion, default templates, CLI model registry, and low consumption mode

## Why nrflow

- **Built for repeatability** — define workflows once, then run the same implementation process across tickets and projects
- **Built for supervision** — keep the orchestration state even when a human needs to step in and drive the agent directly
- **Built for self-hosting** — keep execution, prompts, runtime state, and project access under your own control
- **Built for mixed-agent teams** — use different agent backends and models without changing the workflow model

## How It Works

1. Pick a ticket-scoped or project-scoped workflow from the web UI.
2. nrflow starts agents by layer, running same-layer agents concurrently.
3. Agents write findings that downstream agents can consume in their prompts.
4. If a verifier finds a problem, it can callback an earlier layer and re-run the workflow from that point.
5. If automation gets stuck or needs direction, you can switch to interactive control in the browser.
6. On success, nrflow merges worktree changes, can invoke a conflict resolver, and reports the final workflow result.

## Tech Stack

| Layer | Technologies |
|-------|-------------|
| **Backend** | Go 1.25, Cobra CLI, SQLite (modernc.org/sqlite), gorilla/websocket, golang-migrate, creack/pty |
| **Frontend** | React 19, TypeScript 5.9, TanStack Query, Zustand, Tailwind CSS v4, xterm.js, React Flow, CodeMirror 6, Zod |
| **Database** | SQLite (`~/.nrflow/nrflow.data`), auto-migrating schema |

## Quick Start

```bash
make build && make install
nrflow_server serve
# Open http://localhost:6587
```

To make the server accessible on the local network:

```bash
nrflow_server serve --host 0.0.0.0
```

## CLI Overview

nrflow ships two binaries:

| Binary | Purpose |
|--------|---------|
| `nrflow_server` | HTTP API + WebSocket + Unix socket server |
| `nrflow` | Agent CLI (used by spawned agents) + ticket/dependency management |

**Agent commands** (used by spawned agents via Unix socket):

| Command | Description |
|---------|-------------|
| `nrflow agent fail` | Report agent failure |
| `nrflow agent continue` | Signal continuation |
| `nrflow agent callback --level N` | Trigger callback to re-run an earlier layer |
| `nrflow findings add key:value` | Write findings to current session |
| `nrflow findings append key:value` | Append to existing finding |
| `nrflow findings get [agent-type] [key]` | Read own or cross-agent findings |

**Ticket management** (requires running server):

| Command | Description |
|---------|-------------|
| `nrflow tickets list` | List tickets (filterable by status, type, parent) |
| `nrflow tickets create --title "..."` | Create a ticket |
| `nrflow tickets update <id>` | Update ticket fields |
| `nrflow tickets close <id>` | Close a ticket |
| `nrflow deps add <ticket> <blocker>` | Add a dependency |
| `nrflow deps remove <ticket> <blocker>` | Remove a dependency |

See [agent_manual.md](agent_manual.md) for the full agent definition reference.

## Workflows

Workflows are stored in the database and fully customizable via the web UI. Example configurations:

| Workflow | Phases (by layer) | Use Case |
|----------|-------------------|----------|
| `feature` | L0: setup-analyzer &rarr; L1: test-writer &rarr; L2: implementor &rarr; L3: qa-verifier &rarr; L4: doc-updater | New features (full TDD) |
| `bugfix` | L0: setup-analyzer &rarr; L1: implementor &rarr; L2: qa-verifier | Bug fixes |
| `hotfix` | L0: implementor | Urgent fixes |
| `docs` | L0: setup-analyzer &rarr; L1: doc-updater | Documentation only |
| `refactor` | L0: setup-analyzer &rarr; L1: implementor &rarr; L2: qa-verifier | Code refactoring |

All agents in the same layer run concurrently. The next layer starts only after the current layer completes (at least one agent must pass). If a layer has multiple agents, the next layer must have exactly one agent (fan-in rule).

## Architecture

```mermaid
graph LR
    UI[Web UI] -->|HTTP / WebSocket| Server
    Server -->|Spawn| Agents[Agent Processes]
    Agents -->|Unix Socket| Server
    Server -->|SQLite| DB[(~/.nrflow/nrflow.data)]

    subgraph Server
        API[HTTP API]
        WS[WebSocket Hub]
        Spawner
        Orchestrator
        Socket[Unix Socket]
    end
```

The server runs everything in-process: the orchestrator groups phases by layer, the spawner launches agent processes, and the WebSocket hub broadcasts real-time updates to connected clients. Agent definitions (prompts, models, timeouts) and workflow definitions are stored in the database and managed through the web UI.

## Build & Test

| Target | Description |
|--------|-------------|
| `make build` | Build both binaries (dev, includes UI) |
| `make build-release` | Optimized release build |
| `make install` | Install to `/usr/local/bin` (override with `PREFIX=...`) |
| `make test` | Run backend tests |
| `make test-ui` | Run frontend tests |
| `make test-pkg PKG=...` | Run tests for a single backend package |
| `make clean` | Remove build artifacts |
| `make tidy` | Tidy Go module dependencies |
| `make help` | Show all available targets |

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `NRFLOW_HOME` | `~/.nrflow` | Data directory (database, logs) |
| `NRFLOW_PROJECT` | — | Project identifier (discovered from env) |

Logs are written to `$NRFLOW_HOME/logs/be.log`.

## License

nrflow is source-available under the Business Source License 1.1 (`BUSL-1.1`).

You may use nrflow in production, including self-hosted internal/company
deployments, but you may not offer nrflow to third parties as a hosted or
managed service.

Commercial licenses are available: anderfredx@gmail.com

Each released version converts to Apache License 2.0 on the earlier of:

- April 4, 2030
- the fourth anniversary of that version's first public release

See [LICENSE](LICENSE) for the exact terms.
