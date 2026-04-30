ALTER TABLE agent_sessions DROP COLUMN system_prompt;
ALTER TABLE agent_sessions RENAME COLUMN prompt TO prompt_context;
