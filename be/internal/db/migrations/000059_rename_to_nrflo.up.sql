-- Rename product nrflow -> nrflo in seeded DB content.
-- Rewrites literal "nrflow" occurrences in readonly default templates and in
-- all system agent definition prompts so newly installed prompts use the new
-- CLI binary name. User-created (non-readonly) templates are left alone.

UPDATE default_templates
SET template = REPLACE(template, 'nrflow', 'nrflo'),
    default_template = CASE
        WHEN default_template IS NULL THEN NULL
        ELSE REPLACE(default_template, 'nrflow', 'nrflo')
    END,
    updated_at = '2026-04-19T00:00:00Z'
WHERE readonly = 1
  AND (template LIKE '%nrflow%' OR default_template LIKE '%nrflow%');

UPDATE system_agent_definitions
SET prompt = REPLACE(prompt, 'nrflow', 'nrflo'),
    updated_at = '2026-04-19T00:00:00Z'
WHERE prompt LIKE '%nrflow%';
