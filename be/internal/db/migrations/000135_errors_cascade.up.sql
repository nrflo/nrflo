-- errors.project_id FK was ON DELETE NO ACTION, which blocked project deletion
-- whenever the project had recorded any errors. Recreate with ON DELETE CASCADE
-- to match every other project-child table.
PRAGMA foreign_keys = OFF;

CREATE TABLE errors_new (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    error_type TEXT NOT NULL,
    instance_id TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

INSERT INTO errors_new SELECT * FROM errors;

DROP TABLE errors;
ALTER TABLE errors_new RENAME TO errors;

CREATE INDEX idx_errors_project_id ON errors(project_id);
CREATE INDEX idx_errors_created_at ON errors(created_at);
CREATE INDEX idx_errors_error_type ON errors(error_type);

PRAGMA foreign_keys = ON;
