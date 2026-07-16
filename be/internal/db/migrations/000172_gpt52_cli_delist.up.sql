-- Codex CLI does not run GPT-5.2: app-server model/list does not advertise it
-- and a live request is rejected for ChatGPT-account auth ("The 'gpt-5.2'
-- model is not supported when using Codex with a ChatGPT account"). Keep the
-- row for API mode only and move CLI-mode references to gpt-5.4 (advertised,
-- same effort list low..xhigh, same default effort medium — no effort
-- stamping needed).
UPDATE models SET
    cli_model = '', cli_efforts = '[]', updated_at = '2026-07-16T00:00:00Z'
WHERE id = 'gpt-5.2';

UPDATE agent_definitions SET model = 'gpt-5.4'
WHERE model = 'gpt-5.2' AND execution_mode = 'cli_interactive';
UPDATE agent_definitions SET low_consumption_model = 'gpt-5.4'
WHERE low_consumption_model = 'gpt-5.2' AND execution_mode = 'cli_interactive';
UPDATE system_agent_definitions SET model = 'gpt-5.4'
WHERE model = 'gpt-5.2' AND execution_mode = 'cli_interactive';

UPDATE workflows SET observer_model = 'gpt-5.4'
WHERE observer_model = 'gpt-5.2' AND observer_provider = 'codex';
