CREATE TABLE IF NOT EXISTS errors (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    error_type TEXT NOT NULL,
    instance_id TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(id)
);
CREATE INDEX idx_errors_project_id ON errors(project_id);
CREATE INDEX idx_errors_created_at ON errors(created_at);
CREATE INDEX idx_errors_error_type ON errors(error_type);
