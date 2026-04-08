-- Add layer column to agent_definitions
ALTER TABLE agent_definitions ADD COLUMN layer INTEGER NOT NULL DEFAULT 0;

-- Migrate layer data from workflows.phases JSON to agent_definitions.layer
-- For each workflow, parse its phases JSON and update matching agent_definitions
UPDATE agent_definitions SET layer = COALESCE(
    (SELECT json_extract(je.value, '$.layer')
     FROM workflows w, json_each(w.phases) je
     WHERE LOWER(w.project_id) = LOWER(agent_definitions.project_id)
       AND LOWER(w.id) = LOWER(agent_definitions.workflow_id)
       AND LOWER(json_extract(je.value, '$.agent')) = LOWER(agent_definitions.id)
     LIMIT 1),
    0
);

-- Drop phases column from workflows (SQLite 3.35+ supports ALTER TABLE DROP COLUMN)
ALTER TABLE workflows DROP COLUMN phases;
