DROP INDEX IF EXISTS idx_tool_dispatches_lookup;

ALTER TABLE tool_dispatches RENAME TO nrvapp_tool_dispatches;

CREATE INDEX IF NOT EXISTS idx_nrvapp_tool_dispatches_lookup
    ON nrvapp_tool_dispatches (project_id, tool_name, created_at);
