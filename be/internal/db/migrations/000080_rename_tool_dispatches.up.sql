ALTER TABLE nrvapp_tool_dispatches RENAME TO tool_dispatches;

DROP INDEX IF EXISTS idx_nrvapp_tool_dispatches_lookup;

CREATE INDEX IF NOT EXISTS idx_tool_dispatches_lookup
    ON tool_dispatches (project_id, tool_name, created_at);
