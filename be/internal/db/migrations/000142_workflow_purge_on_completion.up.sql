-- Per-workflow toggle: when enabled, the workflow's sensitive trace data
-- (agent prompts/messages, findings, artifacts, errors, caller input) is purged
-- once a run reaches a terminal state, leaving only a redacted, bare-minimal record.
-- The flag on `workflows` is the UI-editable source of truth; it is snapshotted onto
-- each `workflow_instances` row at creation so the terminal-state hook reads the
-- intent-at-start and the orphan-stop path needs no definition lookup.
ALTER TABLE workflows ADD COLUMN purge_on_completion INTEGER NOT NULL DEFAULT 0;
ALTER TABLE workflow_instances ADD COLUMN purge_on_completion INTEGER NOT NULL DEFAULT 0;
