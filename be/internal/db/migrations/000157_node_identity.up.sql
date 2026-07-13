-- Split execution identity from template identity. node_id identifies the
-- running slot in a workflow instance; agent_type keeps identifying the
-- agent_definitions template it was spawned from. Backfilled from phase,
-- which already holds the execution-slot id for every existing row.
ALTER TABLE agent_sessions ADD COLUMN node_id TEXT NOT NULL DEFAULT '';
UPDATE agent_sessions SET node_id = phase;
CREATE INDEX IF NOT EXISTS idx_agent_sessions_wfi_node ON agent_sessions(workflow_instance_id, node_id);

-- node_role marks agent_definitions rows that are templates only (planner,
-- fanout_template) and must never auto-execute as a workflow phase, mirroring
-- the existing consultant exclusion.
ALTER TABLE agent_definitions ADD COLUMN node_role TEXT NOT NULL DEFAULT 'static';
