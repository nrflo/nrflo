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
│              (~/.nrflo/nrflo.data)                         │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  CONFIG                                                              │
│    project_id    TEXT NOT NULL DEFAULT ''                            │
│    key           TEXT NOT NULL                                       │
│    value         TEXT NOT NULL                                       │
│    PRIMARY KEY (project_id, key)                                     │
│                                                                      │
│  PROJECTS                                                            │
│    id            TEXT PRIMARY KEY                                    │
│    name          TEXT NOT NULL                                       │
│    root_path     TEXT                                                │
│    default_workflow TEXT                                             │
│    default_branch TEXT                                               │
│    use_git_worktrees INTEGER NOT NULL DEFAULT 0                      │
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
│    worktree_path   TEXT               (git worktree path, nullable)  │
│    branch_name     TEXT               (git branch name, nullable)    │
│    endless_loop    INTEGER NOT NULL DEFAULT 0                        │
│                    (project scope; auto-restart on success)          │
│    stop_endless_loop_after_iteration INTEGER NOT NULL DEFAULT 0      │
│                    (graceful stop toggle for endless_loop)           │
│    scheduled_task_id TEXT          (FK → scheduled_tasks.id          │
│                    ON DELETE SET NULL; set by scheduler, NULL for    │
│                    UI/API-triggered runs; mig 000088)                │
│    created_at      TEXT NOT NULL                                     │
│    updated_at      TEXT NOT NULL                                     │
│    INDEX idx_wfi_lookup (project_id, ticket_id, workflow_id,         │
│          scope_type) — non-unique, for query performance             │
│    INDEX idx_workflow_instances_scheduled (scheduled_task_id,        │
│          mig 000088)                                                 │
│    (idx_wfi_ticket_unique dropped by migration 000040 to allow       │
│     multiple instances per ticket+workflow)                          │
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
│    config        TEXT NOT NULL DEFAULT '' (safety settings JSON)     │
│    started_at    TEXT                (when agent started running)    │
│    ended_at      TEXT                (when agent finished)           │
│    spawn_token   TEXT                (per-session HTTP bearer token; │
│                                       set by spawner, NULL on legacy │
│                                       rows; UNIQUE INDEX, mig 000087)│
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
│    groups        TEXT NOT NULL DEFAULT '[]' (JSON: tag groups)      │
│    close_ticket_on_complete INTEGER NOT NULL DEFAULT 1               │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│    PRIMARY KEY (project_id, id)                                      │
│    (phases column dropped by migration 000053; phases are now       │
│     derived from agent_definitions.layer at read time)               │
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
│    low_consumption_model TEXT NOT NULL DEFAULT '' (model override for low consumption mode) │
│    layer         INTEGER NOT NULL DEFAULT 0 (phase execution layer;  │
│                  added by migration 000053, migrated from            │
│                  workflows.phases JSON)                              │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│    PRIMARY KEY (project_id, workflow_id, id)                         │
│    FK (project_id, workflow_id) → workflows(project_id, id) CASCADE │
│                                                                      │
│  SYSTEM_AGENT_DEFINITIONS                                            │
│    id            TEXT PRIMARY KEY                                    │
│    role          TEXT NOT NULL DEFAULT ''                            │
│                  (logical role; backfilled to id for legacy rows)    │
│    execution_mode TEXT NOT NULL DEFAULT 'cli'                        │
│                  CHECK (execution_mode IN ('cli', 'api'))            │
│    model         TEXT NOT NULL DEFAULT 'sonnet'                      │
│    timeout       INTEGER NOT NULL DEFAULT 20                         │
│    prompt        TEXT NOT NULL DEFAULT ''                            │
│    tools         TEXT NOT NULL DEFAULT '' (CSV builtin/HTTP names)   │
│    api_max_iterations INTEGER (NULL = runner default)                │
│    restart_threshold INTEGER                                         │
│    max_fail_restarts INTEGER                                         │
│    stall_start_timeout_sec INTEGER                                   │
│    stall_running_timeout_sec INTEGER                                 │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│    UNIQUE INDEX idx_system_agent_role_mode (role, execution_mode)    │
│    (migration 000063: role backfilled = id for pre-existing rows;    │
│     context-saver-api seeded with role=context-saver, mode=api)     │
│                                                                      │
│  DEFAULT_TEMPLATES                                               │
│    id            TEXT PRIMARY KEY                                    │
│    name          TEXT NOT NULL                                       │
│    type          TEXT NOT NULL DEFAULT 'agent'                       │
│    template      TEXT NOT NULL                                       │
│    readonly      INTEGER NOT NULL DEFAULT 0                          │
│    default_template TEXT          (original text for readonly,       │
│                                    NULL for user-created)            │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│    INDEX idx_default_templates_type (type)                           │
│    (6 readonly agent templates seeded by migration 000042,           │
│     default_template populated by migration 000050,                  │
│     type column + injectable templates added by 000054,
│     continuation injectable removed by 000056,
│     baseline refreshed by 000058)  │
│                                                                      │
│  CLI_MODELS                                                          │
│    id              TEXT PRIMARY KEY                                   │
│    cli_type        TEXT NOT NULL                                      │
│                    CHECK (cli_type IN ('claude','opencode','codex'))  │
│    display_name    TEXT NOT NULL                                      │
│    mapped_model    TEXT NOT NULL                                      │
│    reasoning_effort TEXT NOT NULL DEFAULT ''                          │
│    context_length  INTEGER NOT NULL DEFAULT 200000                   │
│    read_only       INTEGER NOT NULL DEFAULT 0                        │
│    enabled         INTEGER NOT NULL DEFAULT 1                        │
│    created_at      TEXT NOT NULL                                     │
│    updated_at      TEXT NOT NULL                                     │
│    (13 readonly models seeded by migrations 000043 + 000051 + 000057)          │
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
│    started_at    TEXT           (set on first running transition)     │
│    completed_at  TEXT           (set on terminal status transition)   │
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
│  ERRORS                                                              │
│    id            TEXT PRIMARY KEY                                    │
│    project_id    TEXT NOT NULL (FK → projects.id)                   │
│    error_type    TEXT NOT NULL (agent|workflow|system)               │
│    instance_id   TEXT NOT NULL (agent_session.id or wfi.id)         │
│    message       TEXT NOT NULL                                       │
│    created_at    TEXT NOT NULL                                       │
│    INDEX idx_errors_project_id (project_id)                          │
│    INDEX idx_errors_created_at (created_at)                          │
│    INDEX idx_errors_error_type (error_type)                          │
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
│  SCHEDULED_TASKS                                      (mig 000084)   │
│    id              TEXT PRIMARY KEY                                  │
│    project_id      TEXT NOT NULL (FK → projects.id, CASCADE)         │
│    name            TEXT NOT NULL                                     │
│    description     TEXT NOT NULL DEFAULT ''                          │
│    cron_expression TEXT NOT NULL                                     │
│    workflows       TEXT NOT NULL DEFAULT '[]' (JSON: workflow names) │
│    workflow_chain_ids TEXT NOT NULL DEFAULT '[]' (JSON: chain IDs)  │
│    enabled         INTEGER NOT NULL DEFAULT 1                        │
│    last_triggered_at TEXT        (nullable, RFC3339Nano)             │
│    next_run_at     TEXT          (nullable, RFC3339Nano)             │
│    created_at      TEXT NOT NULL                                     │
│    updated_at      TEXT NOT NULL                                     │
│    INDEX idx_scheduled_tasks_project (project_id)                   │
│    INDEX idx_scheduled_tasks_enabled (enabled)                       │
│                                                                      │
│  SCHEDULE_RUNS                                        (mig 000084)   │
│    id                TEXT PRIMARY KEY                                │
│    scheduled_task_id TEXT NOT NULL (FK → scheduled_tasks.id CASCADE) │
│    project_id        TEXT NOT NULL                                   │
│    triggered_at      TEXT NOT NULL                                   │
│    status            TEXT NOT NULL DEFAULT 'running'                 │
│                      (pending|triggered|running|failed)              │
│    workflows         TEXT NOT NULL DEFAULT '[]'                      │
│                      (JSON: [{workflow, instance_id, error}])        │
│    chain_runs        TEXT NOT NULL DEFAULT '[]'                      │
│                      (JSON: [{chain_id, chain_run_id, error}])       │
│    error             TEXT NOT NULL DEFAULT ''                        │
│    INDEX idx_schedule_runs_task (scheduled_task_id, triggered_at)   │
│    INDEX idx_schedule_runs_project (project_id)                      │
│                                                                      │
│  NOTIFICATION_CHANNELS                                               │
│    id          TEXT PRIMARY KEY                                      │
│    project_id  TEXT NOT NULL (FK → projects.id, CASCADE)             │
│    name        TEXT NOT NULL                                         │
│    kind        TEXT NOT NULL CHECK (kind IN ('slack','telegram'))    │
│    enabled     INTEGER NOT NULL DEFAULT 1                            │
│    config      TEXT NOT NULL DEFAULT '{}' (JSON: secrets masked)    │
│    event_types TEXT NOT NULL DEFAULT '[]' (JSON: watched event list) │
│    created_at  TEXT NOT NULL                                         │
│    updated_at  TEXT NOT NULL                                         │
│    INDEX idx_notification_channels_project (project_id)              │
│                                                                      │
│  NOTIFICATION_DELIVERIES                                             │
│    id              TEXT PRIMARY KEY                                  │
│    channel_id      TEXT NOT NULL (FK → notification_channels.id CASCADE)│
│    project_id      TEXT NOT NULL                                     │
│    event_type      TEXT NOT NULL                                     │
│    payload         TEXT NOT NULL DEFAULT '{}' (JSON: raw event data) │
│    status          TEXT NOT NULL DEFAULT 'pending'                   │
│                    CHECK (pending|sent|failed|giving_up)             │
│    attempts        INTEGER NOT NULL DEFAULT 0                        │
│    last_error      TEXT NOT NULL DEFAULT ''                          │
│    next_attempt_at TEXT           (nullable, RFC3339Nano)            │
│    created_at      TEXT NOT NULL                                     │
│    updated_at      TEXT NOT NULL                                     │
│    INDEX idx_notification_deliveries_status (status, next_attempt_at)│
│    INDEX idx_notification_deliveries_channel (channel_id, created_at DESC)│
│                                                                      │
│  REVIEW_ITEMS                          (mig 000072; renamed 000079)  │
│    id            TEXT PRIMARY KEY (rev-xxxxxx)                       │
│    project_id    TEXT NOT NULL (FK → projects.id CASCADE)            │
│    tool_name     TEXT NOT NULL                                       │
│    session_id    TEXT           (nullable agent session reference)   │
│    input         TEXT NOT NULL  (tool input JSON)                    │
│    output        TEXT           (tool output; nullable)              │
│    draft         TEXT           (human-edited draft; nullable)       │
│    status        TEXT NOT NULL DEFAULT 'pending'                     │
│                  CHECK (pending|approved|rejected)                   │
│    reject_reason TEXT           (nullable)                           │
│    created_at    TEXT NOT NULL                                       │
│    updated_at    TEXT NOT NULL                                       │
│    approved_at   TEXT           (nullable, set on Approve)           │
│    INDEX idx_review_items_lookup (project_id, status, created_at DESC)│
│                                                                      │
│  TOOL_DISPATCHES                       (mig 000073; renamed 000080)  │
│    id            TEXT PRIMARY KEY (disp-xxxxxx)                     │
│    project_id    TEXT NOT NULL (FK → projects.id CASCADE)            │
│    session_id    TEXT           (nullable)                           │
│    tool_name     TEXT NOT NULL                                       │
│    input         TEXT NOT NULL                                       │
│    output        TEXT           (nullable)                           │
│    status        TEXT NOT NULL CHECK (success|error)                 │
│    error_msg     TEXT           (nullable)                           │
│    duration_ms   INTEGER NOT NULL                                    │
│    created_at    TEXT NOT NULL                                       │
│    INDEX idx_tool_dispatches_lookup (project_id, tool_name, created_at)│
│                                                                      │
│  CUSTOMER_CONFIG_VERSIONS              (mig 000074; renamed 000081)  │
│    id            INTEGER PRIMARY KEY AUTOINCREMENT                   │
│    project_id    TEXT NOT NULL (FK → projects.id CASCADE)            │
│    file          TEXT NOT NULL (config file path/name)               │
│    version       INTEGER NOT NULL (auto-incremented per project+file)│
│    content       BLOB NOT NULL                                       │
│    actor         TEXT           (nullable, who made the change)      │
│    created_at    TEXT NOT NULL                                       │
│    UNIQUE INDEX idx_customer_config_versions_unique (project_id, file, version)│
│                                                                      │
│  USERS                                                (mig 000075)   │
│    id                  TEXT PRIMARY KEY                               │
│    email               TEXT NOT NULL UNIQUE COLLATE NOCASE            │
│    display_name        TEXT NOT NULL DEFAULT ''                       │
│    password_hash       TEXT NOT NULL (Argon2id PHC format)            │
│    role                TEXT NOT NULL CHECK (admin|viewer)             │
│    status              TEXT NOT NULL CHECK (active|disabled)          │
│    must_change_password INTEGER NOT NULL DEFAULT 0                    │
│    system              INTEGER NOT NULL DEFAULT 0 (mig 000086)        │
│                        usr_admin_seed is flagged system=1             │
│    created_at          TEXT NOT NULL                                  │
│    updated_at          TEXT NOT NULL                                  │
│    last_login_at       TEXT (nullable)                                │
│    INDEX idx_users_status (status)                                    │
│                                                                      │
│  SESSIONS                                             (mig 000076)   │
│    token   TEXT PRIMARY KEY (SCS session token)                      │
│    data    BLOB NOT NULL   (session payload)                          │
│    expiry  REAL NOT NULL   (unix timestamp)                           │
│    INDEX sessions_expiry_idx (expiry)                                 │
│                                                                      │
│  AUDIT_LOG                                            (mig 000077)   │
│    id            TEXT PRIMARY KEY                                     │
│    user_id       TEXT (FK → users.id ON DELETE SET NULL, nullable)    │
│    action        TEXT NOT NULL                                        │
│    resource_type TEXT NOT NULL DEFAULT ''                             │
│    resource_id   TEXT NOT NULL DEFAULT ''                             │
│    ip            TEXT NOT NULL DEFAULT ''                             │
│    user_agent    TEXT NOT NULL DEFAULT ''                             │
│    metadata      TEXT NOT NULL DEFAULT '{}'                           │
│    created_at    TEXT NOT NULL                                        │
│    INDEX idx_audit_log_user (user_id)                                 │
│    INDEX idx_audit_log_created (created_at DESC)                      │
│                                                                      │
│  WORKFLOW_CHAINS                                      (mig 000082)   │
│    id          TEXT NOT NULL                                          │
│    project_id  TEXT NOT NULL (FK → projects.id CASCADE)              │
│    name        TEXT NOT NULL                                          │
│    description TEXT NOT NULL DEFAULT ''                               │
│    created_at  TEXT NOT NULL                                          │
│    updated_at  TEXT NOT NULL                                          │
│    PRIMARY KEY (project_id, id)                                       │
│                                                                      │
│  WORKFLOW_CHAIN_STEPS                                 (mig 000082)   │
│    id                     TEXT PRIMARY KEY                            │
│    project_id             TEXT NOT NULL                               │
│    chain_id               TEXT NOT NULL                               │
│    position               INTEGER NOT NULL                            │
│    workflow_name          TEXT NOT NULL                               │
│    scope_type             TEXT NOT NULL CHECK (project|ticket)        │
│    base_instructions      TEXT NOT NULL DEFAULT ''                    │
│    require_ticket_handoff INTEGER NOT NULL DEFAULT 0                  │
│    created_at             TEXT NOT NULL                               │
│    updated_at             TEXT NOT NULL                               │
│    FK (project_id, chain_id) → workflow_chains(project_id, id) CASCADE│
│    UNIQUE (chain_id, position)                                        │
│    INDEX idx_workflow_chain_steps_chain (chain_id, position)          │
│                                                                      │
│  WORKFLOW_CHAIN_RUNS                                  (mig 000082)   │
│    id                   TEXT PRIMARY KEY                              │
│    project_id           TEXT NOT NULL (FK → projects.id CASCADE)     │
│    chain_id             TEXT NOT NULL                                 │
│    status               TEXT NOT NULL CHECK (pending|running|completed|failed|canceled)│
│    initial_instructions TEXT NOT NULL DEFAULT ''                      │
│    triggered_by         TEXT NOT NULL DEFAULT ''                      │
│    current_position     INTEGER NOT NULL DEFAULT 0                    │
│    started_at           TEXT (nullable)                               │
│    completed_at         TEXT (nullable)                               │
│    created_at           TEXT NOT NULL                                 │
│    updated_at           TEXT NOT NULL                                 │
│    FK (project_id, chain_id) → workflow_chains(project_id, id) CASCADE│
│    INDEX idx_workflow_chain_runs_status (project_id, status)          │
│                                                                      │
│  WORKFLOW_CHAIN_RUN_STEPS                             (mig 000083)   │
│    id                   TEXT PRIMARY KEY                              │
│    chain_run_id         TEXT NOT NULL (FK → workflow_chain_runs.id CASCADE)│
│    position             INTEGER NOT NULL                              │
│    workflow_name        TEXT NOT NULL                                 │
│    scope_type           TEXT NOT NULL                                 │
│    workflow_instance_id TEXT (nullable)                               │
│    ticket_id            TEXT (nullable)                               │
│    instructions_used    TEXT NOT NULL DEFAULT ''                      │
│    require_ticket_handoff INTEGER NOT NULL DEFAULT 0                  │
│    status               TEXT NOT NULL CHECK (pending|running|completed|failed|skipped|canceled)│
│    started_at           TEXT (nullable)                               │
│    ended_at             TEXT (nullable)                               │
│    created_at           TEXT NOT NULL                                 │
│    updated_at           TEXT NOT NULL                                 │
│    INDEX idx_workflow_chain_run_steps_run (chain_run_id, position)    │
│                                                                      │
│  PYTHON_SCRIPTS                               (mig 000085)           │
│    id          TEXT PRIMARY KEY (ps-xxxxxx)                           │
│    project_id  TEXT NOT NULL (FK → projects.id CASCADE)              │
│    name        TEXT NOT NULL                                          │
│    description TEXT NOT NULL DEFAULT ''                               │
│    code        TEXT NOT NULL DEFAULT ''                               │
│    created_at  TEXT NOT NULL                                          │
│    updated_at  TEXT NOT NULL                                          │
│    UNIQUE INDEX python_scripts_project_id_id (project_id, id)        │
│                                                                      │
│  AGENT_DEFINITIONS (mig 000085 additions)                            │
│    python_script_id TEXT (nullable, FK reference to python_scripts;  │
│                     no DB FK constraint — application-level only)    │
│    execution_mode CHECK now includes 'script' (cli|api|script)       │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Adding a Database Migration

Current highest migration: **000088** (workflow_instance_scheduled — adds `workflow_instances.scheduled_task_id` column + index `idx_workflow_instances_scheduled` to link runs to their originating scheduled task)

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
