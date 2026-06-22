-- Opus 4.6/4.7/4.8 and Sonnet 4.6 are 1M-context-native on the Anthropic API.
-- The api_models seeds shipped them at 200000, which made api-mode agents trip
-- the low-context save/relaunch dance ~5x too early. Correct to 1000000.
--
-- Haiku 4.5 stays at 200000; the *_1m rows are already 1000000. cli_models is
-- intentionally NOT touched: for Claude Code the bare id legitimately uses 200k
-- and "[1m]" opts into 1M.
UPDATE api_models
SET context_length = 1000000, updated_at = '2026-06-22T00:00:00Z'
WHERE provider = 'anthropic' AND id IN ('opus_4_6', 'opus_4_7', 'opus_4_8', 'sonnet');
