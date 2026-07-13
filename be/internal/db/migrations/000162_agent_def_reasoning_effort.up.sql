-- reasoning_effort is a per-def override of the model row's reasoning
-- effort ("", low, medium, high, xhigh, max, ultra). NULL = inherit from the
-- cli_models/api_models row selected by the def's model; a non-NULL value
-- (including '') overrides it, re-validated at spawn time against the
-- def's current model row so a model swap cannot leave a stale illegal
-- override in place. Nullable, backfill-safe: existing rows default to NULL.
ALTER TABLE agent_definitions ADD COLUMN reasoning_effort TEXT;
ALTER TABLE system_agent_definitions ADD COLUMN reasoning_effort TEXT;
