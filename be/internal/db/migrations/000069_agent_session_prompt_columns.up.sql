-- Rename agent_sessions.prompt_context → prompt (it stores the rendered user
-- prompt, not the system suffix; the old name was misleading) and add a new
-- system_prompt column to record the system-prompt-suffix we deliver to the
-- agent. Both columns make it possible to reproduce a spawn from the DB row
-- alone, without having to reconstruct the templates.

ALTER TABLE agent_sessions RENAME COLUMN prompt_context TO prompt;
ALTER TABLE agent_sessions ADD COLUMN system_prompt TEXT;
