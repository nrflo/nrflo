INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at) VALUES
    ('continuation', 'Continuation (stall/fail restart)',
     '## Continuation

Your previous run was interrupted and did not save state. Resume work from a clean slate; re-read prior findings with `nrflow findings get` if you need them.
',
     '## Continuation

Your previous run was interrupted and did not save state. Resume work from a clean slate; re-read prior findings with `nrflow findings get` if you need them.
',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
