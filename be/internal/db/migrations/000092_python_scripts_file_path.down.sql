CREATE TABLE python_scripts_backup (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    code        TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
INSERT INTO python_scripts_backup SELECT id, project_id, name, description, code, created_at, updated_at FROM python_scripts;
DROP TABLE python_scripts;
ALTER TABLE python_scripts_backup RENAME TO python_scripts;
CREATE UNIQUE INDEX python_scripts_project_id_id ON python_scripts (project_id, id);
