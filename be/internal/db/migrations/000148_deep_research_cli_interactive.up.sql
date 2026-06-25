-- deep-research ships as cli_interactive agents: the claude/codex CLIs
-- self-authenticate, so the workflow needs no server-side Anthropic/OpenAI API
-- credential (the previous api-mode agents failed to spawn unless a key was put
-- in the global project's env or the server env). Flip already-seeded copies:
--   * execution_mode api -> cli_interactive (all 6 agents)
--   * verify_b -> the codex GPT-5.5 cli-model (was the OpenAI api-model gpt54_high)
--   * replace the api-mode "Then stop." completion cue with the explicit cli call
-- Fresh installs get this straight from the seed (service/deep_research_seed*.go);
-- these UPDATEs only touch installs seeded before the change (0 rows otherwise).
UPDATE agent_definitions
   SET execution_mode = 'cli_interactive',
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research';

UPDATE agent_definitions
   SET model = 'codex_gpt55_high',
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'verify_b';

UPDATE agent_definitions
   SET prompt = REPLACE(prompt, 'Then stop.', 'Then call agent_finished.')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research';
