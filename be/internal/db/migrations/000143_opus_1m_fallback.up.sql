-- Give every 1M-context Opus CLI model a single-step --fallback-model to its
-- 200k counterpart. When the 1M model is server-side overloaded (HTTP 529),
-- the Claude CLI now falls back to the 200k model on its own retries instead
-- of hammering the overloaded model until the turn fails. The value is the
-- raw CLI model string (passed verbatim to --fallback-model), not the nrflo id.
UPDATE cli_models SET fallback_models = 'claude-opus-4-6' WHERE id = 'opus_4_6_1m' AND fallback_models = '';
UPDATE cli_models SET fallback_models = 'claude-opus-4-7' WHERE id = 'opus_4_7_1m' AND fallback_models = '';
UPDATE cli_models SET fallback_models = 'claude-opus-4-8' WHERE id = 'opus_4_8_1m' AND fallback_models = '';
