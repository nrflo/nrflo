-- Per-session running token/cost accounting (debounced flush, no per-call
-- writes) plus the daily rollup column.
ALTER TABLE agent_sessions ADD COLUMN tokens_json TEXT;
ALTER TABLE agent_sessions ADD COLUMN cost_estimate REAL;

ALTER TABLE daily_stats ADD COLUMN cost_estimate REAL NOT NULL DEFAULT 0;
