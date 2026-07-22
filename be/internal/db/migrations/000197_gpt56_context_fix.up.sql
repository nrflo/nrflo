-- Codex 0.144.6 corrected the Sol/Terra/Luna bundled window to 272000 (was
-- 372000); api_context needs no change here — gpt-5.6-sol is already
-- 1050000 via 000170, and terra/luna are left per ticket.
UPDATE models SET cli_context = 272000, updated_at = '2026-07-23T00:00:00Z'
WHERE id IN ('gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna');
