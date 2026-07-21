-- Nullable release_date (ISO YYYY-MM-DD) drives newest-release-first sort in
-- the console model picker (service.SortModelsForPicker). No CHECK, so a
-- plain ADD COLUMN suffices (no table rebuild). NULL = unknown, sorts last.
ALTER TABLE models ADD COLUMN release_date TEXT;

UPDATE models SET release_date = '2026-07-20' WHERE id = 'fable-5';
UPDATE models SET release_date = '2026-07-18' WHERE id IN ('opus-4-8', 'opus-4-8-1m');
UPDATE models SET release_date = '2026-07-10' WHERE id IN ('opus-4-7', 'opus-4-7-1m');
UPDATE models SET release_date = '2026-06-01' WHERE id IN ('opus-4-6', 'opus-4-6-1m');
UPDATE models SET release_date = '2026-07-15' WHERE id = 'sonnet-5';
UPDATE models SET release_date = '2026-07-01' WHERE id = 'haiku-4-5';
UPDATE models SET release_date = '2026-07-16' WHERE id IN ('gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna');
UPDATE models SET release_date = '2026-06-15' WHERE id IN ('gpt-5.5', 'gpt-5.5-mini');
UPDATE models SET release_date = '2026-05-15' WHERE id IN ('gpt-5.4', 'gpt-5.4-mini');
UPDATE models SET release_date = '2026-04-15' WHERE id = 'gpt-5.3-codex';
UPDATE models SET release_date = '2026-03-15' WHERE id = 'gpt-5.2';
