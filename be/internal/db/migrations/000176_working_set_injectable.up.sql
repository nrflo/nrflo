-- Seed the `working-set` injectable template, expanded into console
-- UserPromptSubmit additionalContext by spawner.WorkingSetInjector. Empty
-- body ⇒ renderInjectable returns "" ⇒ fully backward-silent no-op.
INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at) VALUES
    ('working-set', 'Working set', '', '', 1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
