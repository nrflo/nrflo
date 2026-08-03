package spawner

import (
	"path/filepath"

	"be/internal/repo"
)

// workspaceContextBlock resolves and renders the readonly workspace-context
// injectable describing where this agent's checkout lives: the live project
// tree (workspace-live-tree) or an nrflo-owned isolated worktree
// (workspace-worktree). Classification compares Config.ProjectRoot — the
// single seam every backend derives cwd from, including delegate workers'
// per-delegation worktree (delegate.go:206) — against projects.root_path.
// Returns "" when ProjectRoot is unset ("" or ".") or the project lookup
// fails, so a spawn never fails on this seam.
func (s *Spawner) workspaceContextBlock(projectID string) string {
	root := s.config.ProjectRoot
	if root == "" || root == "." {
		return ""
	}

	pool := s.pool()
	if pool == nil {
		return ""
	}

	project, err := repo.NewProjectRepo(pool, s.config.Clock).Get(projectID)
	if err != nil || !project.RootPath.Valid || project.RootPath.String == "" {
		return ""
	}

	id := "workspace-worktree"
	if filepath.Clean(root) == filepath.Clean(project.RootPath.String) {
		id = "workspace-live-tree"
	}

	return s.expandInjectable(id, map[string]string{"WORK_ROOT": root})
}
