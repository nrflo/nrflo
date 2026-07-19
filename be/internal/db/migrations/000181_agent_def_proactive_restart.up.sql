-- proactive_restart_threshold_tokens is a per-def override of the watcher-
-- triggered proactive restart-with-digest policy: NULL = inherit the global
-- proactive_restart_threshold_default config value, 0 = disabled (no
-- proactive restart for this def), >0 = the ledger-token ceiling that, once
-- crossed at an idle task boundary, triggers a restart carrying a digest of
-- the session's progress. Nullable, backfill-safe: existing rows default to
-- NULL. system_agent_definitions gets no column — system agents always
-- resolve through the nil path to the global default.
ALTER TABLE agent_definitions ADD COLUMN proactive_restart_threshold_tokens INTEGER;
