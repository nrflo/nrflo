-- Seed the `api-system-prompt` injectable with the exact text of the
-- defaultAPISystemPrompt constant (spawner/spawner_types.go) so a fresh DB
-- renders a byte-identical api-mode system prompt.
INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at) VALUES
    ('api-system-prompt', 'API system prompt',
     'You are an agent in a workflow. Follow the instructions below.',
     'You are an agent in a workflow. Follow the instructions below.',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
