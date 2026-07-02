-- Sub-workflow support: workflows gain an admin-set callable_as_subworkflow flag
-- (only flagged defs can be started by the run_subworkflow agent tool), and
-- workflow_instances gain a persisted launch_depth — the unified nesting depth
-- counter incremented by both run_subworkflow and next_workflow_on_success.
-- Persisting the depth (instead of the old in-memory ChainDepth) makes the cap
-- survive retry-failed / continue, which previously reset it to 0.
ALTER TABLE workflows ADD COLUMN callable_as_subworkflow INTEGER NOT NULL DEFAULT 0;
ALTER TABLE workflow_instances ADD COLUMN launch_depth INTEGER NOT NULL DEFAULT 0;

-- The bundled deep-research workflow is the first callable: the deleted
-- web_deep_research builtin is replaced by run_subworkflow(workflow="deep-research").
UPDATE workflows SET callable_as_subworkflow = 1 WHERE project_id = '__global__' AND id = 'deep-research';
