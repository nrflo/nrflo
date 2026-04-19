-- Reverse rename nrflo -> nrflow in seeded DB content.
-- Mirror of up migration; only rewrites readonly templates and system agent
-- prompts. Safe against re-runs.

UPDATE default_templates
SET template = REPLACE(template, 'nrflo', 'nrflow'),
    default_template = CASE
        WHEN default_template IS NULL THEN NULL
        ELSE REPLACE(default_template, 'nrflo', 'nrflow')
    END,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE readonly = 1
  AND (template LIKE '%nrflo%' OR default_template LIKE '%nrflo%');

UPDATE system_agent_definitions
SET prompt = REPLACE(prompt, 'nrflo', 'nrflow'),
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE prompt LIKE '%nrflo%';
