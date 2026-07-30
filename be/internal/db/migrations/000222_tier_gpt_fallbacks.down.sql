DELETE FROM tier_models WHERE tier IN (2, 3, 4) AND position = 2 AND model_id = 'gpt-5.6-terra';

UPDATE system_agent_definitions SET model = 'sonnet-5', updated_at = datetime('now')
WHERE id IN ('_t1_executor', 'planner-system', 'planner-system-api', 'conflict-resolver') AND model = '';
UPDATE system_agent_definitions SET model = 'haiku-4-5', updated_at = datetime('now')
WHERE id IN ('spec-normalizer', 'context-saver-api', 'context-saver') AND model = '';

DELETE FROM tier_models WHERE tier = 5;
