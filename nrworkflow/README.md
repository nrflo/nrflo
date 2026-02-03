# nrworkflow - Go CLI

Unified CLI tool for ticket management and AI agent orchestration. This is the Go implementation of nrworkflow, providing all functionality in a single binary.

## Installation

```bash
cd ~/projects/2026/nrworkflow/nrworkflow
make build

# Install globally
sudo cp nrworkflow /usr/local/bin/

# Or create symlink (recommended for development)
sudo ln -s $(pwd)/nrworkflow /usr/local/bin/nrworkflow
```

## Quick Start

```bash
# Initialize the database
nrworkflow init-db

# Create a project
nrworkflow project create myproject --name "My Project"

# Create a ticket
nrworkflow ticket create -p myproject --title "Add user authentication" \
  -d "Implement OAuth2 login" --type=feature

# List tickets
nrworkflow ticket list -p myproject
nrworkflow ticket list -p myproject --status=open --type=bug

# Show ticket details
nrworkflow ticket show MYPROJECT-001 -p myproject

# Initialize workflow on ticket
nrworkflow init MYPROJECT-001 -p myproject -w feature

# Check workflow status
nrworkflow status MYPROJECT-001 -p myproject

# Spawn an agent
nrworkflow agent spawn setup-analyzer MYPROJECT-001 -p myproject \
  --session=$SESSION_MARKER -w feature
```

## Project Structure

```
nrworkflow/
├── cmd/nrworkflow/main.go       # Entry point
├── internal/
│   ├── cli/                     # Cobra commands
│   │   ├── root.go              # Root command, global flags (-p, -D)
│   │   ├── project.go           # project create/list/show/delete
│   │   ├── ticket.go            # ticket create/list/show/update/close/...
│   │   ├── workflow.go          # workflows, init, status, progress, get, set
│   │   ├── agent.go             # agent spawn/preview/list/active/...
│   │   ├── findings.go          # findings add/get
│   │   ├── serve.go             # HTTP API server
│   │   └── init_db.go           # Database initialization
│   ├── spawner/                 # Agent spawner
│   │   └── spawner.go           # Spawn and monitor agents
│   ├── api/                     # HTTP API
│   │   ├── server.go            # Server setup, CORS
│   │   ├── handlers_tickets.go  # Ticket endpoints
│   │   └── handlers_workflow.go # Workflow endpoints
│   ├── config/                  # Configuration
│   │   └── config.go            # Server config loading
│   ├── db/                      # Database layer
│   │   └── db.go                # SQLite connection, schema
│   ├── model/                   # Data models
│   │   ├── project.go           # Project model
│   │   ├── ticket.go            # Ticket model
│   │   └── agent_session.go     # Agent session model
│   ├── repo/                    # Repository pattern
│   │   ├── project.go           # Project CRUD
│   │   ├── ticket.go            # Ticket CRUD
│   │   ├── dependency.go        # Dependency management
│   │   └── agent_session.go     # Agent session tracking
│   └── id/                      # ID generation
│       └── generator.go         # Ticket ID generator
├── go.mod
├── go.sum
└── Makefile
```

## Commands

### Global Flags

```bash
-p, --project    Project ID (required for most commands)
-D, --data       Path to database file (default: ~/projects/2026/nrworkflow/nrworkflow.data)

# Or use environment variables:
NRWORKFLOW_PROJECT=myproject           # Override project
NRWORKFLOW_HOME=/path/to/nrworkflow    # Override database location
```

### Project Management

```bash
nrworkflow project create <id> --name "Name"
nrworkflow project list
nrworkflow project show <id>
nrworkflow project delete <id>
```

### Ticket Management

```bash
nrworkflow ticket create -p <project> --title "..." --type feature -d "..."
nrworkflow ticket list -p <project> [--status open] [--type bug] [--json]
nrworkflow ticket show <id> -p <project> [--json]
nrworkflow ticket update <id> -p <project> [--title ...] [--status ...]
nrworkflow ticket close <id> -p <project> [--reason "..."]
nrworkflow ticket delete <id> -p <project>
nrworkflow ticket search <query> -p <project> [--json]
nrworkflow ticket ready -p <project>
nrworkflow ticket status -p <project> [--json]
nrworkflow ticket dep add <child> <parent> -p <project>
```

### Workflow Management

```bash
nrworkflow workflows                              # List available workflows
nrworkflow init <ticket> -p <project> -w <workflow>
nrworkflow status <ticket> -p <project> [-w <name>]
nrworkflow progress <ticket> -p <project> [-w <name>] [--json]
nrworkflow get <ticket> -p <project> [-w <name>] [field]
nrworkflow set <ticket> -p <project> -w <name> <key> <value>
```

### Phase Management

```bash
nrworkflow phase start <ticket> <phase> -p <project> -w <name>
nrworkflow phase complete <ticket> <phase> pass|fail|skipped -p <project> -w <name>
nrworkflow phase ready <ticket> <phase> -p <project> -w <name>
```

### Agent Management

```bash
nrworkflow agent list
nrworkflow agent spawn <type> <ticket> -p <project> --session=<uuid> -w <workflow>
nrworkflow agent preview <type> <ticket> -p <project> [-w <name>]
nrworkflow agent active <ticket> -p <project> -w <name>
nrworkflow agent complete <ticket> <type> -p <project> -w <name>
nrworkflow agent fail <ticket> <type> -p <project> -w <name>
nrworkflow agent kill <ticket> -p <project> -w <name> [--model=...]
nrworkflow agent retry <ticket> -p <project> -w <name>
```

### Findings Management

```bash
nrworkflow findings add <ticket> <agent> <key> <value> -p <project> -w <name>
nrworkflow findings get <ticket> <agent> -p <project> -w <name> [key]
```

### HTTP API Server

```bash
nrworkflow serve              # Default port 6587
nrworkflow serve --port=8080  # Custom port
```

## Database

SQLite database at `~/projects/2026/nrworkflow/nrworkflow.data` (single global database for all projects) with tables:
- **projects**: Project definitions
- **tickets**: Tickets with project_id foreign key
- **dependencies**: Ticket dependencies
- **agent_sessions**: Agent session tracking
- **tickets_fts**: Full-text search index

### Schema

```sql
-- Projects
CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    root_path TEXT,
    default_workflow TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Tickets (composite primary key)
CREATE TABLE tickets (
    id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'open',
    priority INTEGER NOT NULL DEFAULT 2,
    issue_type TEXT NOT NULL DEFAULT 'task',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    closed_at TEXT,
    created_by TEXT NOT NULL,
    close_reason TEXT,
    agents_state TEXT,
    PRIMARY KEY (project_id, id),
    FOREIGN KEY (project_id) REFERENCES projects(id)
);
```

## Configuration

Workflow and agent configuration is loaded from `~/projects/2026/nrworkflow/config.json`:

```json
{
  "cli": {
    "default": "claude"
  },
  "agents": {
    "setup-analyzer": {"model": "sonnet", "max_turns": 50, "timeout": 15},
    "implementor": {"model": "opus", "max_turns": 80, "timeout": 30}
  },
  "workflows": {
    "feature": {
      "description": "Full TDD workflow",
      "phases": [
        {"id": "investigation", "agent": "setup-analyzer"},
        {"id": "implementation", "agent": "implementor"}
      ]
    }
  }
}
```

## HTTP API

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/projects` | List projects |
| `GET` | `/api/v1/projects/:id` | Get project |
| `GET` | `/api/v1/tickets` | List tickets (requires project) |
| `GET` | `/api/v1/tickets/:id` | Get ticket |
| `POST` | `/api/v1/tickets` | Create ticket |
| `PUT` | `/api/v1/tickets/:id` | Update ticket |
| `POST` | `/api/v1/tickets/:id/close` | Close ticket |
| `DELETE` | `/api/v1/tickets/:id` | Delete ticket |
| `GET` | `/api/v1/tickets/:id/workflow` | Get workflow state |
| `PUT` | `/api/v1/tickets/:id/workflow` | Update workflow state |
| `GET` | `/api/v1/tickets/:id/agents` | Get agent sessions |
| `GET` | `/api/v1/search?q=` | Full-text search |
| `POST` | `/api/v1/dependencies` | Add dependency |
| `DELETE` | `/api/v1/dependencies` | Remove dependency |
| `GET` | `/api/v1/status` | Dashboard summary |

Project is specified via:
- `X-Project` header
- `?project=` query parameter

## Building

```bash
# Build
make build

# Build release (optimized)
make build-release

# Install to /usr/local/bin
sudo make install

# Clean
make clean
```

## Dependencies

- Go 1.21+
- github.com/spf13/cobra - CLI framework
- modernc.org/sqlite - Pure Go SQLite (no CGO)
- github.com/google/uuid - UUID generation
