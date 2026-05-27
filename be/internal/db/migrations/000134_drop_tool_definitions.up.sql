-- Remove the HTTP tools feature: the tool_definitions table and its indices.
-- Agent tools are now builtin + python only; per-agent selection uses the
-- agent_definitions.tools CSV. tool_dispatches is shared with python tools and
-- is intentionally kept.

DROP INDEX IF EXISTS idx_tool_definitions_workflow;
DROP INDEX IF EXISTS idx_tool_definitions_project;
DROP TABLE IF EXISTS tool_definitions;
