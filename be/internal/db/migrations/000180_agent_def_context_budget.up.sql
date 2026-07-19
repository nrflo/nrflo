-- context_budget_tokens is a per-def override of the context watcher's live
-- token budget (api mode only): NULL = inherit the global context_budget_default
-- config value, 0 = disabled (no budget-triggered GC for this def), >0 = the
-- token ceiling that triggers selective epoch GC. Nullable, backfill-safe:
-- existing rows default to NULL. system_agent_definitions gets no column —
-- system agents always resolve through the nil path to the global default.
ALTER TABLE agent_definitions ADD COLUMN context_budget_tokens INTEGER;
