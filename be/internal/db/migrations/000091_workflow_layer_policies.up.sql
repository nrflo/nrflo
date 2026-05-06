CREATE TABLE IF NOT EXISTS workflow_layer_policies (
    project_id  TEXT    NOT NULL,
    workflow_id TEXT    NOT NULL,
    layer       INTEGER NOT NULL,
    pass_policy TEXT    NOT NULL DEFAULT 'any',
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL,
    PRIMARY KEY (project_id, workflow_id, layer),
    FOREIGN KEY (project_id, workflow_id) REFERENCES workflows (project_id, id) ON DELETE CASCADE
);
