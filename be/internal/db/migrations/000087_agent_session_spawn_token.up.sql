ALTER TABLE agent_sessions ADD COLUMN spawn_token TEXT;
CREATE UNIQUE INDEX idx_agent_sessions_spawn_token
    ON agent_sessions(spawn_token) WHERE spawn_token IS NOT NULL;
