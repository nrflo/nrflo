-- Sub-workflow hardening.
-- parent_instance_id: the run_subworkflow caller's instance id ('' for
-- top-level and chain runs) — drives result-readback authorization and the
-- skip of next_workflow_on_success successors for sub-runs.
-- subworkflow_depth: nesting via run_subworkflow only (chain hops keep it),
-- so next-on-success chains no longer consume the sub-workflow depth budget.
-- subworkflow_starts: persisted invocation budget on the parent row, so it
-- survives pause/continue and retry-failed (in-memory runState is recreated).
ALTER TABLE workflow_instances ADD COLUMN parent_instance_id TEXT NOT NULL DEFAULT '';
ALTER TABLE workflow_instances ADD COLUMN subworkflow_depth INTEGER NOT NULL DEFAULT 0;
ALTER TABLE workflow_instances ADD COLUMN subworkflow_starts INTEGER NOT NULL DEFAULT 0;

-- The web_deep_research builtin was replaced by run_subworkflow/get_subworkflow;
-- a tools-CSV token matching no tool hard-fails the spawn, so rewrite existing
-- agent definitions that granted it (with and without a space after the comma).
UPDATE agent_definitions SET tools =
  TRIM(REPLACE(REPLACE(',' || tools || ',', ',web_deep_research,', ',run_subworkflow,get_subworkflow,'), ', web_deep_research,', ',run_subworkflow,get_subworkflow,'), ',')
WHERE ',' || REPLACE(tools, ' ', '') || ',' LIKE '%,web_deep_research,%';
