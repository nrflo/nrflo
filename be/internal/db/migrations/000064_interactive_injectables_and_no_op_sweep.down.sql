-- Remove the two new injectable templates added in the up migration.
-- UPDATE rewrites (context-saver prompt) are one-way and are NOT reverted.
DELETE FROM default_templates WHERE id IN ('system-prompt-suffix', 'finish-reminder');
