CREATE TABLE IF NOT EXISTS daily_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL,
    date TEXT NOT NULL,
    tickets_created INTEGER NOT NULL DEFAULT 0,
    tickets_closed INTEGER NOT NULL DEFAULT 0,
    tokens_spent INTEGER NOT NULL DEFAULT 0,
    agent_time_sec REAL NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    UNIQUE(project_id, date),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_daily_stats_project_date ON daily_stats(project_id, date);
