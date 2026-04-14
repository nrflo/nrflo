ALTER TABLE default_templates ADD COLUMN type TEXT NOT NULL DEFAULT 'agent';

UPDATE default_templates SET type = 'agent';

CREATE INDEX IF NOT EXISTS idx_default_templates_type ON default_templates(type);

INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at) VALUES
    ('continuation',      'Continuation (stall/fail restart)',
     '## Continuation

Your previous run was interrupted and did not save state. Resume work from a clean slate; re-read prior findings with `nrflow findings get` if you need them.
',
     '## Continuation

Your previous run was interrupted and did not save state. Resume work from a clean slate; re-read prior findings with `nrflow findings get` if you need them.
',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('low-context',       'Low-context restart',
     '## Continuation From Saved State

Your previous run was interrupted at low context. Below is the summary it saved:


',
     '## Continuation From Saved State

Your previous run was interrupted at low context. Below is the summary it saved:


',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('callback',          'Callback instructions',
     '## Callback Instructions

This agent is being re-run due to a callback from **${CALLBACK_FROM_AGENT}**.

_No callback instructions_
',
     '## Callback Instructions

This agent is being re-run due to a callback from **${CALLBACK_FROM_AGENT}**.

_No callback instructions_
',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('user-instructions', 'User instructions',
     '## User Instructions

_No user instructions provided_
',
     '## User Instructions

_No user instructions provided_
',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
