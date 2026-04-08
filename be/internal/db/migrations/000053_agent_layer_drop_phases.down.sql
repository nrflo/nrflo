-- Add phases column back to workflows
ALTER TABLE workflows ADD COLUMN phases TEXT NOT NULL DEFAULT '[]';

-- Rebuild phases JSON from agent_definitions layer data
UPDATE workflows SET phases = COALESCE(
    (SELECT json_group_array(json_object('agent', ad.id, 'layer', ad.layer))
     FROM agent_definitions ad
     WHERE LOWER(ad.project_id) = LOWER(workflows.project_id)
       AND LOWER(ad.workflow_id) = LOWER(workflows.id)),
    '[]'
);

-- Drop layer column from agent_definitions
ALTER TABLE agent_definitions DROP COLUMN layer;
