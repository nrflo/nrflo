-- Anticipated Claude subscription reset time (RFC3339) parsed from the statusline
-- rate_limits payload. When a 5h/7d window is near-exhausted, a later rate-limited
-- restart waits until this exact reset instead of guessing an exponential backoff.
-- Distinct from rate_limit_until_ts (the computed retry time written on exit).
ALTER TABLE agent_sessions ADD COLUMN rate_limit_reset_ts TEXT;
