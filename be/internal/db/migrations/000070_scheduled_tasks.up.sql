CREATE TABLE IF NOT EXISTS scheduled_tasks (
    id             TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    cron_expression TEXT NOT NULL,
    workflows      TEXT NOT NULL DEFAULT '[]',
    enabled        INTEGER NOT NULL DEFAULT 1,
    last_triggered_at TEXT,
    next_run_at    TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_project ON scheduled_tasks (project_id);
CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_enabled ON scheduled_tasks (enabled);

CREATE TABLE IF NOT EXISTS schedule_runs (
    id                 TEXT PRIMARY KEY,
    scheduled_task_id  TEXT NOT NULL REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
    project_id         TEXT NOT NULL,
    triggered_at       TEXT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'running',
    workflows          TEXT NOT NULL DEFAULT '[]',
    error              TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_schedule_runs_task ON schedule_runs (scheduled_task_id, triggered_at);
CREATE INDEX IF NOT EXISTS idx_schedule_runs_project ON schedule_runs (project_id);
