-- Persist the engine name ('claude'|'codex') a console_chat row was started
-- with, so the list endpoint does not depend on the in-memory chatSession map.
ALTER TABLE agent_sessions ADD COLUMN console_engine TEXT DEFAULT NULL;
