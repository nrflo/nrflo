-- Observability footprint for tier/system-agent activity.
--
-- agent_sessions gains six columns recording what actually resolved at
-- spawn time (tier, resolved provider/execution_mode/effort, chain
-- position, and the fallback entries tried before the winner).
--
-- refinery_runs is a durable, append-only log of every fold attempt
-- (success and failure). Its slot key is dual: session_id identifies a
-- console fold, (workflow_instance_id, node_id) identifies an autonomous
-- fold — never both populated meaningfully at once. It deliberately has NO
-- foreign key on session_id/workflow_instance_id: agent_sessions and
-- workflow_instances cascade-delete, but the fold footprint must outlive
-- them for observability.
ALTER TABLE agent_sessions ADD COLUMN tier INTEGER;
ALTER TABLE agent_sessions ADD COLUMN resolved_provider TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_sessions ADD COLUMN resolved_execution_mode TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_sessions ADD COLUMN resolved_effort TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_sessions ADD COLUMN chain_position INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_sessions ADD COLUMN fallback_from TEXT;

CREATE TABLE refinery_runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL DEFAULT '',
	workflow_instance_id TEXT NOT NULL DEFAULT '',
	node_id TEXT NOT NULL DEFAULT '',
	project_id TEXT NOT NULL DEFAULT '',
	provider TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	prompt_tokens INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL CHECK (status IN ('ok', 'failed')),
	error TEXT NOT NULL DEFAULT '',
	fold_count INTEGER NOT NULL DEFAULT 0,
	folded_at TEXT NOT NULL
);

CREATE INDEX idx_refinery_runs_folded_at ON refinery_runs (folded_at DESC);
CREATE INDEX idx_agent_sessions_tier_created ON agent_sessions (tier, created_at DESC);
