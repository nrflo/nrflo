-- Per-session cumulative timing-bucket accounting (thinking, tool-arg
-- streaming, text, tool wait seconds), debounced flush mirroring 000184's
-- tokens_json/cost_estimate columns.
ALTER TABLE agent_sessions ADD COLUMN time_buckets_json TEXT;
