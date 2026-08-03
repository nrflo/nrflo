-- Seed two readonly `injectable` default_templates rows describing where a
-- spawned agent's checkout lives, appended to the rendered prompt body
-- (spawner/template_workspace.go workspaceContextBlock, template.go
-- loadTemplate) so agents stop inferring a branch from the ticket-id commit
-- convention. Both share a reporting rule: never create/switch branches, and
-- if a report names a branch/commit, read it from git rather than deriving
-- it from the ticket id.

INSERT INTO default_templates (id, name, template, default_template, readonly, type, created_at, updated_at) VALUES
    ('workspace-live-tree', 'Workspace Context (Live Tree)',
     '## Workspace

You work directly in the project''s live checkout at `${WORK_ROOT}`, on whatever branch it currently has checked out — nrflo created no branch for this run.

- Never create or switch branches.
- If your report names a branch or commit, read it from git (`git branch --show-current`, `git rev-parse --short HEAD`) — never derive a branch name from the ticket id.
- Omit the branch entirely when you did not read it from git.
',
     '## Workspace

You work directly in the project''s live checkout at `${WORK_ROOT}`, on whatever branch it currently has checked out — nrflo created no branch for this run.

- Never create or switch branches.
- If your report names a branch or commit, read it from git (`git branch --show-current`, `git rev-parse --short HEAD`) — never derive a branch name from the ticket id.
- Omit the branch entirely when you did not read it from git.
',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('workspace-worktree', 'Workspace Context (Worktree)',
     '## Workspace

You work in an isolated git worktree at `${WORK_ROOT}` that nrflo created and whose branch nrflo owns and merges — do not name or assume that branch.

- Never create or switch branches.
- If your report names a branch or commit, read it from git (`git branch --show-current`, `git rev-parse --short HEAD`) — never derive a branch name from the ticket id.
- Omit the branch entirely when you did not read it from git.
',
     '## Workspace

You work in an isolated git worktree at `${WORK_ROOT}` that nrflo created and whose branch nrflo owns and merges — do not name or assume that branch.

- Never create or switch branches.
- If your report names a branch or commit, read it from git (`git branch --show-current`, `git rev-parse --short HEAD`) — never derive a branch name from the ticket id.
- Omit the branch entirely when you did not read it from git.
',
     1, 'injectable', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
