-- The delegate worktree contract is server-owned commits (CommitAndCollect),
-- but the workspace-worktree injectable never said so — workers briefed to
-- "commit your work" obeyed, and the finalize pass used to treat their clean
-- tree as "nothing landed" and branch -D'd their commits into dangling
-- objects. CommitAndCollect now keys Committed off the branch HEAD, and this
-- states the no-commit rule in the prompt itself.

UPDATE default_templates
SET template = REPLACE(template,
	'- Never create or switch branches.',
	'- Never create or switch branches.
- Never run `git commit` — nrflo commits your work onto its branch when the delegation ends; leave your changes uncommitted in the tree.'),
    default_template = REPLACE(default_template,
	'- Never create or switch branches.',
	'- Never create or switch branches.
- Never run `git commit` — nrflo commits your work onto its branch when the delegation ends; leave your changes uncommitted in the tree.'),
    updated_at = CURRENT_TIMESTAMP
WHERE id = 'workspace-worktree';
