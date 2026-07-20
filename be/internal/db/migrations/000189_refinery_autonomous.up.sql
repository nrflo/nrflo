-- Autonomous refinery sidecar: sibling head table to refinery_digests, keyed
-- by (workflow_instance_id, node_id) so a digest survives a kill->relaunch
-- chain (the slot is identical across relaunches, unlike session id). Seeds
-- the global refinery_autonomous_enabled gate ON — default-ON read semantics
-- (val != 'false') mean this seed only matters for a future settings UI.

CREATE TABLE IF NOT EXISTS refinery_autonomous_digests (
    workflow_instance_id TEXT    NOT NULL,
    node_id              TEXT    NOT NULL,
    project_id           TEXT    NOT NULL,
    version              INTEGER NOT NULL DEFAULT 0,
    content              TEXT    NOT NULL DEFAULT '',
    fold_count           INTEGER NOT NULL DEFAULT 0,
    created_at           TEXT    NOT NULL,
    updated_at           TEXT    NOT NULL,
    PRIMARY KEY (workflow_instance_id, node_id),
    FOREIGN KEY (project_id) REFERENCES projects (id) ON DELETE CASCADE
);

INSERT OR IGNORE INTO config (project_id, key, value) VALUES ('', 'refinery_autonomous_enabled', 'true');
