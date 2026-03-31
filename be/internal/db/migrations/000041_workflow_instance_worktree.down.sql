-- SQLite supports DROP COLUMN since 3.35.0 (2021-03-12).
ALTER TABLE workflow_instances DROP COLUMN worktree_path;
ALTER TABLE workflow_instances DROP COLUMN branch_name;
