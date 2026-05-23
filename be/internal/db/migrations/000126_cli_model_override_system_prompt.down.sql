DELETE FROM default_templates WHERE id = 'system-prompt';
ALTER TABLE cli_models DROP COLUMN override_system_prompt;
