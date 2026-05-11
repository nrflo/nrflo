DELETE FROM agent_definitions WHERE workflow_id = '__spec_import__' AND id = 'spec-normalizer';
DELETE FROM workflows WHERE id = '__spec_import__';
DELETE FROM system_agent_definitions WHERE id = 'spec-normalizer';
