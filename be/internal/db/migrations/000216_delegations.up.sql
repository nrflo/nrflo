-- Durable delegation tracking, replacing the transient `_delegation_<id>`
-- workflow_instance finding. A row is written once at fanout seed time and
-- never deleted — only marked completed/failed and consumed — so a caller's
-- GetDelegation poll and a later trace/UI feature can both read it back.
--
-- Deliberately NO foreign keys on caller_session_id/workflow_instance_id:
-- agent_sessions and workflow_instances cascade-delete, but the delegation
-- footprint must outlive them for observability (same rationale as
-- refinery_runs, migration 000198).
CREATE TABLE delegations (
	id TEXT PRIMARY KEY,
	caller_session_id TEXT NOT NULL DEFAULT '',
	workflow_instance_id TEXT NOT NULL DEFAULT '',
	project_id TEXT NOT NULL DEFAULT '',
	tier TEXT NOT NULL DEFAULT '',
	brief TEXT NOT NULL DEFAULT '',
	fanout INTEGER NOT NULL DEFAULT 1,
	worker_session_ids TEXT NOT NULL DEFAULT '[]',
	spawn_errors TEXT NOT NULL DEFAULT '[]',
	depth INTEGER NOT NULL DEFAULT 0,
	fanout_done INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed')) DEFAULT 'running',
	created_at TEXT NOT NULL,
	completed_at TEXT,
	consumed_at TEXT
);

CREATE INDEX idx_delegations_caller_session ON delegations (caller_session_id);
CREATE INDEX idx_delegations_created_at ON delegations (created_at DESC);
