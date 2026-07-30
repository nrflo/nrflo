UPDATE system_agent_definitions SET
    model = 'haiku-4-5',
    updated_at = datetime('now')
WHERE id = '_refinery';
