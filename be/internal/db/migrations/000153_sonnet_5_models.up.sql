-- Claude Sonnet 5 replaces Sonnet 4.6 as the model behind the "sonnet" id.
-- Unlike Opus, Sonnet has no separate 200k/1M variant rows -- the single
-- "sonnet" id is corrected in place in both cli_models and api_models.
-- Sonnet 5 has a single 1M-token context window (no 200k mode), so both
-- tables move to context_length=1000000; cli_models previously tracked 200k
-- for Sonnet 4.6.
UPDATE cli_models
SET display_name = 'Sonnet 5', mapped_model = 'claude-sonnet-5', context_length = 1000000, updated_at = '2026-07-01T00:00:00Z'
WHERE id = 'sonnet' AND cli_type = 'claude' AND mapped_model = 'claude-sonnet-4-6';

UPDATE api_models
SET display_name = 'Claude Sonnet 5', mapped_model = 'claude-sonnet-5', context_length = 1000000, updated_at = '2026-07-01T00:00:00Z'
WHERE id = 'sonnet' AND provider = 'anthropic' AND mapped_model = 'claude-sonnet-4-6';
