CREATE TABLE IF NOT EXISTS system_agent_definitions (
    id TEXT PRIMARY KEY,
    model TEXT NOT NULL DEFAULT 'sonnet',
    timeout INTEGER NOT NULL DEFAULT 20,
    prompt TEXT NOT NULL DEFAULT '',
    restart_threshold INTEGER,
    max_fail_restarts INTEGER,
    stall_start_timeout_sec INTEGER,
    stall_running_timeout_sec INTEGER,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
