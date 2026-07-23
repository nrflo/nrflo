-- Stepwise prompt mode: agent_definitions gains prompt_mode ('full' default,
-- unchanged single-shot prompt behavior) and a nullable steps JSON array used
-- only in 'stepwise' mode. agent_step_cursors tracks per-instance progress
-- through that array, keyed by (workflow_instance_id, node_id) rather than
-- session_id — a cursor must survive session restarts/rotations within the
-- same node, since a stepwise agent can be respawned mid-sequence.
ALTER TABLE agent_definitions ADD COLUMN prompt_mode TEXT NOT NULL DEFAULT 'full'
    CHECK (prompt_mode IN ('full', 'stepwise'));
ALTER TABLE agent_definitions ADD COLUMN steps TEXT;

CREATE TABLE IF NOT EXISTS agent_step_cursors (
    workflow_instance_id TEXT    NOT NULL,
    node_id               TEXT    NOT NULL,
    steps_snapshot         TEXT    NOT NULL,
    revision               INTEGER NOT NULL DEFAULT 1,
    current_index          INTEGER NOT NULL DEFAULT 0,
    completed              TEXT    NOT NULL DEFAULT '[]',
    created_at             TEXT    NOT NULL,
    updated_at             TEXT    NOT NULL,
    PRIMARY KEY (workflow_instance_id, node_id),
    FOREIGN KEY (workflow_instance_id) REFERENCES workflow_instances (id) ON DELETE CASCADE
);
