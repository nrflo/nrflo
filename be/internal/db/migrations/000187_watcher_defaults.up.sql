-- Seed the context-watcher tuning knobs at their code defaults so a fresh DB
-- exposes them as discoverable/editable global config rows. Values mirror
-- the hardcoded fallbacks in be/internal/spawner/context_watcher.go and
-- context_restart*.go. Append-only (no down file per project convention).

INSERT OR IGNORE INTO config (project_id, key, value) VALUES ('', 'context_budget_fraction', '0.65');
INSERT OR IGNORE INTO config (project_id, key, value) VALUES ('', 'context_budget_default', '0');
INSERT OR IGNORE INTO config (project_id, key, value) VALUES ('', 'context_decay_turns', '20');
INSERT OR IGNORE INTO config (project_id, key, value) VALUES ('', 'cache_ttl_sec', '300');
INSERT OR IGNORE INTO config (project_id, key, value) VALUES ('', 'min_epoch_interval_calls', '20');
INSERT OR IGNORE INTO config (project_id, key, value) VALUES ('', 'proactive_restart_threshold_default', '250000');
INSERT OR IGNORE INTO config (project_id, key, value) VALUES ('', 'proactive_restart_min_interval_sec', '600');
INSERT OR IGNORE INTO config (project_id, key, value) VALUES ('', 'proactive_restart_max_per_session', '0');
INSERT OR IGNORE INTO config (project_id, key, value) VALUES ('', 'proactive_restart_boundary_window_turns', '10');
INSERT OR IGNORE INTO config (project_id, key, value) VALUES ('', 'proactive_restart_console_pct', '75');
