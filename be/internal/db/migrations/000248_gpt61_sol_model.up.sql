-- CLI context, efforts and default from the Codex model catalog; API settings
-- and per-MTok prices from developers.openai.com/api/docs/models/gpt-6.1-sol.
INSERT INTO models (
    id, provider, display_name, cli_model, api_model, cli_efforts, api_efforts,
    cli_context, api_context, fallback_models, default_effort, read_only,
    enabled, created_at, updated_at, release_date,
    price_in, price_out, price_cache_write, price_cache_read
) VALUES (
    'gpt-6.1-sol', 'openai', 'GPT-6.1 Sol', 'gpt-6.1-sol', 'gpt-6.1-sol',
    '["low","medium","high","xhigh","max","ultra"]',
    '["low","medium","high","xhigh","max"]',
    272000, 1050000, '', 'low', 1, 1,
    '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z', NULL,
    2, 10, 2.5, 0.1
);
