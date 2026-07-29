-- Server-set launch attribution for workflow_instances: which surface started
-- the run and, for console-initiated runs, the launching console session id.
-- Distinct from `parent_session`, which is API-settable and drives the
-- #{PARENT_SESSION} prompt template var injected into spawned agents.
ALTER TABLE workflow_instances ADD COLUMN origin TEXT NOT NULL DEFAULT '';
ALTER TABLE workflow_instances ADD COLUMN origin_session_id TEXT NOT NULL DEFAULT '';
