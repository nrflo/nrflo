-- Seed the claude-limits-refresher system agent definition.
INSERT INTO system_agent_definitions (
    id, role, model, timeout, prompt, execution_mode, stall_start_timeout_sec, stall_running_timeout_sec, created_at, updated_at
) VALUES (
    'claude-limits-refresher',
    'claude-limits-refresher',
    'haiku',
    1,
    'Reply with the single word "exit" and nothing else.',
    'cli',
    30,
    60,
    datetime('now'),
    datetime('now')
);

-- Seed claude-limits-refresh workflow for every existing project.
INSERT OR IGNORE INTO workflows
    (id, project_id, description, scope_type, groups, close_ticket_on_complete, next_workflow_on_success, created_at, updated_at)
SELECT
    'claude-limits-refresh',
    id,
    'Claude limits refresh (internal)',
    'project',
    '[]',
    0,
    '',
    datetime('now'),
    datetime('now')
FROM projects;

-- Seed claude-limits-refresher agent_definition for every existing project.
INSERT OR IGNORE INTO agent_definitions
    (id, project_id, workflow_id, model, timeout, prompt, layer, created_at, updated_at)
SELECT
    'claude-limits-refresher',
    id,
    'claude-limits-refresh',
    'haiku',
    1,
    '',
    0,
    datetime('now'),
    datetime('now')
FROM projects;
