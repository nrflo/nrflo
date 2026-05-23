ALTER TABLE cli_models ADD COLUMN override_system_prompt INTEGER NOT NULL DEFAULT 0;

INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at) VALUES
    ('system-prompt',
     'System prompt (override)',
     'You are a helpful AI assistant. The system-prompt-suffix is still appended automatically after this prompt.',
     'You are a helpful AI assistant. The system-prompt-suffix is still appended automatically after this prompt.',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
