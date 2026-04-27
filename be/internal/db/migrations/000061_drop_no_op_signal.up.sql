-- Remove the obsolete `nrflow findings add no-op:no-op` escape-hatch line from
-- the seeded conflict-resolver and context-saver system agent prompts. The
-- instant-stall detection that consumed this signal has been removed; the
-- finding has no remaining effect.

-- Note: migration 000059 already rewrote `nrflow` -> `nrflo` in all
-- system_agent_definitions prompts, so the patterns below match the
-- post-rename text.

UPDATE system_agent_definitions
SET prompt = REPLACE(
        prompt,
        '
- If the conflict is too complex to resolve confidently, call `nrflo agent fail --reason "description of why"`
- If there is nothing to do, run `nrflo findings add no-op:no-op` before exiting',
        '
- If the conflict is too complex to resolve confidently, call `nrflo agent fail --reason "description of why"`'
    ),
    updated_at = datetime('now')
WHERE id = 'conflict-resolver';

UPDATE system_agent_definitions
SET prompt = REPLACE(
        prompt,
        'Then run these two commands in order:

```bash
NRF_SESSION_ID=${TARGET_SESSION_ID} nrflo findings add to_resume "<your concise summary>"
```

```bash
nrflo findings add no-op:no-op
```',
        'Then run this command:

```bash
NRF_SESSION_ID=${TARGET_SESSION_ID} nrflo findings add to_resume "<your concise summary>"
```'
    ),
    updated_at = datetime('now')
WHERE id = 'context-saver';
