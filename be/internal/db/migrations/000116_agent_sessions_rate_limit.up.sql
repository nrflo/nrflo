ALTER TABLE agent_sessions ADD COLUMN rate_limit_retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_sessions ADD COLUMN rate_limit_until_ts TEXT;
ALTER TABLE agent_sessions ADD COLUMN last_retry_class TEXT;
