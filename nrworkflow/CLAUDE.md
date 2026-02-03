# WFW - Ticket Management CLI

Single-binary CLI tool for ticket management, designed as a replacement for Beads (`bd`) with native Apple Silicon support.

## Build

```bash
# Requires Go (install via: brew install go)
make build-release    # Builds CGO-free ARM64 binary
make install          # Copies to /usr/local/bin/
```

Go binary location: `/opt/homebrew/bin/go`

## Project Structure

```
wfw/
├── cmd/wfw/main.go           # Entry point
├── internal/
│   ├── cli/                  # Command handlers (init, create, list, show, update, close, delete, search, dep, ready)
│   ├── db/                   # SQLite connection, schema, FTS5
│   ├── model/                # Ticket, Dependency structs
│   ├── repo/                 # Repository pattern for CRUD
│   └── id/                   # ID generator (prefix-xxx format)
├── go.mod
├── go.sum
└── Makefile
```

## Database

- File: `wfw.data` (SQLite)
- Location: Searches from CWD upward (like git)
- Custom path: `-D /path/to/wfw.data` or `WFW_DATA=/path/to/wfw.data`
- Schema: `tickets`, `dependencies`, `config` tables + FTS5 for search

```bash
# Use custom location
wfw -D /path/to/wfw.data list
WFW_DATA=/path/to/wfw.data wfw list
```

## CLI Commands

| Command | Usage |
|---------|-------|
| `wfw init` | `wfw init [--prefix project]` |
| `wfw create` | `wfw create --title="..." [--id=REF-123] --type=feature --priority=2 -d "..."` |
| `wfw list` | `wfw list [--status open] [--type bug] [--json]` |
| `wfw show` | `wfw show <id> [--json]` (returns array) |
| `wfw update` | `wfw update <id> [--status ...] [--agents_state ...]` |
| `wfw close` | `wfw close <id> [--reason "..."]` |
| `wfw delete` | `wfw delete <id> [--force]` |
| `wfw search` | `wfw search <query> [--json]` |
| `wfw dep add` | `wfw dep add <child> <parent>` |
| `wfw ready` | `wfw ready [--json]` |
| `wfw status` | `wfw status [--pending 20] [--completed 15] [--json]` |

## nrworkflow.py Integration

The `agents_state` field stores workflow state as JSON:

```bash
wfw show <ticket> --json    # Returns [{ ... "agents_state": "..." }]
wfw update <ticket> --agents_state "<json>"
```

## Key Dependencies

- `github.com/spf13/cobra` - CLI framework
- `modernc.org/sqlite` - Pure Go SQLite (no CGO)
