DROP INDEX IF EXISTS idx_agent_sessions_sibling_origin;
ALTER TABLE agent_sessions DROP COLUMN sibling_origin_session_id;
