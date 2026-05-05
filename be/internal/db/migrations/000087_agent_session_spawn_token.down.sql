DROP INDEX IF EXISTS idx_agent_sessions_spawn_token;
ALTER TABLE agent_sessions DROP COLUMN spawn_token;
