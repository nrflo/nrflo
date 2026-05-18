-- The seeded `haiku` cli_models row was created with mapped_model='haiku',
-- which the Claude CLI accepts as a shorthand but the Anthropic API rejects.
-- The api-mode in-process runner passes mapped_model verbatim to the SDK
-- (`be/internal/spawner/apirun/provider/anthropic/translate.go:18`), so any
-- api-mode agent using model='haiku' — most notably the seeded
-- `context-saver-api` system agent triggered by every low-context relaunch —
-- got a 404 model_not_found from Anthropic.
--
-- Repoint mapped_model to the full Anthropic id. The Claude Code CLI also
-- accepts the full id (the existing opus_4_7 mapping is `claude-opus-4-7`),
-- so cli_interactive spawns keep working.
UPDATE cli_models SET mapped_model = 'claude-haiku-4-5', updated_at = '2026-05-18T00:00:00Z'
WHERE id = 'haiku' AND mapped_model = 'haiku';
