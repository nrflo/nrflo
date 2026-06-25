-- Revert deep-research to the api-mode roster (see the up migration).
UPDATE agent_definitions
   SET prompt = REPLACE(prompt, 'Then call agent_finished.', 'Then stop.')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research';

UPDATE agent_definitions
   SET model = 'gpt54_high',
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research' AND id = 'verify_b';

UPDATE agent_definitions
   SET execution_mode = 'api',
       updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
 WHERE project_id = '__global__' AND workflow_id = 'deep-research';
