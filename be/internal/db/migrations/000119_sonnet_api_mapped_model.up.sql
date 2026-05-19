-- The seeded `sonnet` cli_models row was created with mapped_model='sonnet',
-- which the Claude CLI accepts as a shorthand but the Anthropic API rejects
-- with a 404 model_not_found. The api-mode in-process runner passes
-- mapped_model verbatim to the SDK
-- (`be/internal/spawner/apirun/provider/anthropic/translate.go:18`), so any
-- api-mode agent using model='sonnet' got a 404 from Anthropic.
--
-- Repoint mapped_model to the full Anthropic id. The Claude Code CLI also
-- accepts the full id, so cli_interactive spawns keep working. Mirrors the
-- 000112 fix that did the same for `haiku`.
UPDATE cli_models SET mapped_model = 'claude-sonnet-4-6', updated_at = '2026-05-19T00:00:00Z'
WHERE id = 'sonnet' AND mapped_model = 'sonnet';
