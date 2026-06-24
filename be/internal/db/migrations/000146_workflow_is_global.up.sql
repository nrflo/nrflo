-- Global workflows become first-class via an is_global flag on the workflow
-- row, instead of being inferred from a reserved project_id string. The
-- reserved '__global__' project_id survives only as an internal storage
-- namespace (kept out of project listings); is_global is the authoritative
-- semantic flag that listing, admin gates, and the UI key on.
ALTER TABLE workflows ADD COLUMN is_global INTEGER NOT NULL DEFAULT 0;

-- Flag any rows already living in the reserved storage namespace (the bundled
-- deep-research workflow on existing installs).
UPDATE workflows SET is_global = 1 WHERE LOWER(project_id) = '__global__';
