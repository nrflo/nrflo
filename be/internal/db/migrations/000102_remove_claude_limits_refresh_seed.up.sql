DELETE FROM agent_definitions WHERE workflow_id = 'claude-limits-refresh' AND id = 'claude-limits-refresher';
DELETE FROM workflows WHERE id = 'claude-limits-refresh';
DELETE FROM system_agent_definitions WHERE id = 'claude-limits-refresher';
DELETE FROM scheduled_tasks WHERE workflows LIKE '%"claude-limits-refresh"%';
