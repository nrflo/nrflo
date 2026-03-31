-- Add worktree_path and branch_name to workflow_instances for persistence.
ALTER TABLE workflow_instances ADD COLUMN worktree_path TEXT;
ALTER TABLE workflow_instances ADD COLUMN branch_name TEXT;
