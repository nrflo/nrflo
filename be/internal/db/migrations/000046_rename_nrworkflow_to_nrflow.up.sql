-- Rename nrworkflow references to nrflow in seeded data

-- Update system_agent_definitions prompts
UPDATE system_agent_definitions
SET prompt = REPLACE(prompt, 'nrworkflow', 'nrflow')
WHERE prompt LIKE '%nrworkflow%';

-- Update default_templates templates
UPDATE default_templates
SET template = REPLACE(template, 'nrworkflow', 'nrflow')
WHERE template LIKE '%nrworkflow%';

-- Update agent_definitions prompts
UPDATE agent_definitions
SET prompt = REPLACE(prompt, 'nrworkflow', 'nrflow')
WHERE prompt LIKE '%nrworkflow%';
