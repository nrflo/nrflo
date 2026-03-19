# Database Package

SQLite database layer with connection pooling, auto-migration, and embedded SQL migration files.

## Querier Interface

`db.go` exports a `Querier` interface satisfied by both `*DB` and `*Pool`:
- Methods: `Exec`, `Query`, `QueryRow`, `Begin`
- Repos that don't need pool/DB-specific features accept `db.Querier`
- Enables passing either `*DB` or `*Pool` to the same repo constructor

## Connection Pool

`pool.go` manages the connection pool:
- Max connections: 10
- Max idle connections: 5
- Pure Go SQLite via `modernc.org/sqlite` (no CGO)

## Database Schema

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
│    default_branch TEXT                                               │
│    use_git_worktrees INTEGER NOT NULL DEFAULT 0                      │
│    use_docker_isolation INTEGER NOT NULL DEFAULT 0                   │
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
│    scope_type      TEXT NOT NULL DEFAULT 'ticket'                    │
│                    CHECK (scope_type IN ('ticket', 'project'))       │
│    status          TEXT NOT NULL                                     │
│                    (active|completed|failed|project_completed)       │
│    findings        TEXT NOT NULL      (JSON: workflow-level findings)│
│    skip_tags       TEXT NOT NULL DEFAULT '[]' (JSON: skipped tags)  │
│    retry_count     INTEGER NOT NULL DEFAULT 0                        │
│    parent_session  TEXT               (orchestrating session UUID)   │
│    created_at      TEXT NOT NULL                                     │
│    updated_at      TEXT NOT NULL                                     │
│    INDEX idx_wfi_lookup (project_id, ticket_id, workflow_id,         │
│          scope_type) — non-unique, for query performance             │
│    UNIQUE INDEX idx_wfi_ticket_unique (project_id, ticket_id,        │
│          workflow_id) WHERE scope_type = 'ticket'                    │
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
│    status        TEXT NOT NULL                                       │
│      (running|completed|failed|timeout|continued|project_completed|callback|user_interactive|interactive_completed|skipped)
│    result        TEXT                                                │
│      (pass|fail|continue|timeout|callback|skipped)                  │
│    result_reason TEXT                (explanation for result)        │
│    pid           INTEGER             (OS process ID)                 │
│    findings      TEXT                (JSON: per-agent findings)      │
│    context_left  INTEGER             (remaining context window %)    │
│    ancestor_session_id TEXT          (links continuation chain)      │
│    spawn_command TEXT                (Full CLI command for replay)   │
│    prompt_context TEXT               (System prompt file contents)   │
│    restart_count INTEGER NOT NULL DEFAULT 0  (low-context restarts) │
│    started_at    TEXT                (when agent started running)    │
│    ended_at      TEXT                (when agent finished)           │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│    FK workflow_instance_id → workflow_instances(id) CASCADE          │
│    FK ancestor_session_id → agent_sessions(id) SET NULL             │
│                                                                      │
│  AGENT_MESSAGES                                                      │
│    id            INTEGER PRIMARY KEY AUTOINCREMENT                   │
│    session_id    TEXT NOT NULL  (FK → agent_sessions.id, CASCADE)   │
│    seq           INTEGER NOT NULL    (message sequence number)       │
│    content       TEXT NOT NULL       (message text)                  │
│    category      TEXT NOT NULL DEFAULT 'text'                        │
│                  (text|tool|subagent|skill)                          │
│    created_at    TEXT NOT NULL                                       │
│    INDEX idx_agent_messages_session (session_id, seq)                │
│                                                                      │
│  WORKFLOWS                                                           │
│    id            TEXT NOT NULL                                       │
│    project_id    TEXT NOT NULL  (FK → projects.id)                  │
│    description   TEXT                                                │
│    scope_type    TEXT NOT NULL DEFAULT 'ticket'                      │
│                  CHECK (scope_type IN ('ticket', 'project'))         │
│    phases        TEXT NOT NULL  (JSON array string)                  │
│    groups        TEXT NOT NULL DEFAULT '[]' (JSON: tag groups)      │
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
│    max_fail_restarts INTEGER       (NULL/0 = disabled, >0 = auto-restart on failure) │
│    stall_start_timeout_sec INTEGER (NULL = 120s default, 0 = disabled)│
│    stall_running_timeout_sec INTEGER (NULL = 480s default, 0 = disabled)│
│    tag           TEXT NOT NULL DEFAULT '' (skip-tag assignment)      │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│    PRIMARY KEY (project_id, workflow_id, id)                         │
│    FK (project_id, workflow_id) → workflows(project_id, id) CASCADE │
│                                                                      │
│  SYSTEM_AGENT_DEFINITIONS                                            │
│    id            TEXT PRIMARY KEY                                    │
│    model         TEXT NOT NULL DEFAULT 'sonnet'                      │
│    timeout       INTEGER NOT NULL DEFAULT 20                         │
│    prompt        TEXT NOT NULL DEFAULT ''                            │
│    restart_threshold INTEGER                                         │
│    max_fail_restarts INTEGER                                         │
│    stall_start_timeout_sec INTEGER                                   │
│    stall_running_timeout_sec INTEGER                                 │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│                                                                      │
│  CHAIN_EXECUTIONS                                                    │
│    id            TEXT PRIMARY KEY   (UUID)                            │
│    project_id    TEXT NOT NULL                                        │
│    name          TEXT NOT NULL                                        │
│    status        TEXT NOT NULL DEFAULT 'pending'                      │
│                  CHECK (pending|running|completed|failed|canceled)    │
│    workflow_name TEXT NOT NULL                                        │
│    epic_ticket_id TEXT          (nullable, auto-closed on completion) │
│    created_by    TEXT NOT NULL DEFAULT ''                             │
│    created_at    TEXT NOT NULL                                        │
│    updated_at    TEXT NOT NULL                                        │
│    INDEX (project_id, status)                                        │
│    INDEX (project_id, epic_ticket_id)                                │
│                                                                      │
│  CHAIN_EXECUTION_ITEMS                                               │
│    id                    TEXT PRIMARY KEY (UUID)                      │
│    chain_id              TEXT NOT NULL (FK → chain_executions.id)     │
│    ticket_id             TEXT NOT NULL                                │
│    position              INTEGER NOT NULL                             │
│    status                TEXT NOT NULL DEFAULT 'pending'              │
│                  CHECK (pending|running|completed|failed|skipped|canceled)
│    workflow_instance_id  TEXT           (set when item starts)        │
│    started_at            TEXT                                         │
│    ended_at              TEXT                                         │
│    INDEX (chain_id, position)                                        │
│                                                                      │
│  CHAIN_EXECUTION_LOCKS                                               │
│    project_id  TEXT NOT NULL                                         │
│    ticket_id   TEXT NOT NULL                                         │
│    chain_id    TEXT NOT NULL (FK → chain_executions.id)              │
│    UNIQUE (project_id, ticket_id)                                    │
│    Prevents overlapping ticket runs across pending/running chains    │
│                                                                      │
│  DAILY_STATS                                                         │
│    id             INTEGER PRIMARY KEY AUTOINCREMENT                  │
│    project_id     TEXT NOT NULL (FK → projects.id)                   │
│    date           TEXT NOT NULL (ISO date YYYY-MM-DD)                │
│    tickets_created INTEGER NOT NULL DEFAULT 0                        │
│    tickets_closed  INTEGER NOT NULL DEFAULT 0                        │
│    tokens_spent    INTEGER NOT NULL DEFAULT 0                        │
│    agent_time_sec  REAL NOT NULL DEFAULT 0                           │
│    updated_at      TEXT NOT NULL                                     │
│    UNIQUE(project_id, date)                                          │
│                                                                      │
│  WS_EVENT_LOG                                                        │
│    seq            INTEGER PRIMARY KEY AUTOINCREMENT                  │
│    project_id     TEXT NOT NULL                                      │
│    ticket_id      TEXT NOT NULL DEFAULT ''                            │
│    event_type     TEXT NOT NULL                                      │
│    workflow       TEXT NOT NULL DEFAULT ''                            │
│    payload        TEXT NOT NULL DEFAULT '{}'  (JSON event data)      │
│    created_at     TEXT NOT NULL                                      │
│    INDEX idx_ws_event_log_scope_seq (project_id, ticket_id, seq)    │
│    INDEX idx_ws_event_log_created_at (created_at)                   │
│    Retention: events older than 24h cleaned up hourly                │
│                                                                      │
│  PROJECT_FINDINGS                                                    │
│    project_id  TEXT NOT NULL (FK → projects.id, CASCADE)             │
│    key         TEXT NOT NULL                                         │
│    value       TEXT NOT NULL DEFAULT ''  (JSON-serialized value)     │
│    updated_at  TEXT NOT NULL                                         │
│    PRIMARY KEY (project_id, key)                                     │
│                                                                      │
│  PREFERENCES                                                         │
│    name        TEXT PRIMARY KEY                                      │
│    value       TEXT NOT NULL DEFAULT ''                               │
│    created_at  TEXT NOT NULL                                          │
│    updated_at  TEXT NOT NULL                                          │
│                                                                      │
│  TICKETS_FTS (Full-text search)                                      │
│    project_id, id, title, description                                │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Adding a Database Migration

1. Create `migrations/NNNNNN_description.up.sql` (next sequence number)
2. The up file contains the schema change (e.g. `ALTER TABLE ... ADD COLUMN`)
3. Down migrations are not used — rollbacks are done via new forward migrations
4. Migrations are embedded automatically via `//go:embed *.sql` in `migrations/embed.go`
5. Rebuild: `cd be && make build`
6. Migrations run automatically on server startup — no manual `migrate` command needed
7. **Documentation updates:** Update this file's schema diagram if user-visible

## Files

| File | Purpose |
|------|---------|
| `db.go` | SQLite connection setup, `Querier` interface |
| `pool.go` | Connection pool (10 max, 5 idle) |
| `migrate.go` | Migration runner |
| `migrations/` | SQL files (embedded via `//go:embed`) |
| `migrations/embed.go` | Go embed directive |
