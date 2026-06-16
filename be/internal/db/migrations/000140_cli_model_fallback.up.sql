-- Comma-separated Claude --fallback-model chain (≤3 models) tried in sequence when
-- the primary model is overloaded/unavailable. Empty = no fallback. Claude-only.
ALTER TABLE cli_models ADD COLUMN fallback_models TEXT NOT NULL DEFAULT '';
