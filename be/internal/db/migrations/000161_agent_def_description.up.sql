-- description is a human/planner-facing summary of what a fanout_template
-- agent definition does; required (validated in service layer) for
-- node_role='fanout_template' rows, optional for static/planner rows.
-- Backfill-safe: existing rows default to ''.
ALTER TABLE agent_definitions ADD COLUMN description TEXT NOT NULL DEFAULT '';
