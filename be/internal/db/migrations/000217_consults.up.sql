-- Durable consult tracking, mirroring `delegations` (migration 000216). A row
-- is written once before the consultant is spawned and marked terminal once
-- the answer (or a failure) is known, so consult children — which have no
-- caller column of their own on agent_sessions — gain a stable consult_id and
-- caller linkage for the trace/UI grouping feature.
--
-- Deliberately NO foreign keys on caller_session_id/workflow_instance_id:
-- agent_sessions and workflow_instances cascade-delete, but the consult
-- footprint must outlive them for observability (same rationale as
-- `delegations`, migration 000216).
CREATE TABLE consults (
	id TEXT PRIMARY KEY,
	caller_session_id TEXT NOT NULL DEFAULT '',
	workflow_instance_id TEXT NOT NULL DEFAULT '',
	project_id TEXT NOT NULL DEFAULT '',
	consultant_id TEXT NOT NULL DEFAULT '',
	question TEXT NOT NULL DEFAULT '',
	child_session_id TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed')) DEFAULT 'running',
	error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	completed_at TEXT
);

CREATE INDEX idx_consults_caller_session ON consults (caller_session_id);
CREATE INDEX idx_consults_workflow_instance ON consults (workflow_instance_id);
