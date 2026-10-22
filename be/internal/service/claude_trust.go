package service

import (
	"context"
	"os"

	"be/internal/claudecfg"
	"be/internal/db"
	"be/internal/logger"
)

// TrustClaudeAnchors pre-accepts Claude's folder-trust dialog for the worktree
// base and every project root at startup. Claude honors a trusted ancestor, so
// spawns in these trees survive a concurrent claude process rewriting
// .claude.json and dropping a per-launch entry.
func TrustClaudeAnchors(pool *db.Pool) {
	ctx := context.Background()
	dirs := []string{worktreeBasePath}
	if err := os.MkdirAll(worktreeBasePath, 0o755); err != nil {
		logger.Warn(ctx, "claude trust: create worktree base", "err", err)
	}
	rows, err := pool.Query(`SELECT root_path FROM projects WHERE root_path IS NOT NULL AND root_path != ''`)
	if err != nil {
		logger.Warn(ctx, "claude trust: list project roots", "err", err)
	} else {
		for rows.Next() {
			var root string
			if rows.Scan(&root) == nil {
				dirs = append(dirs, root)
			}
		}
		rows.Close()
	}
	for _, dir := range dirs {
		TrustClaudeFolder(dir)
	}
}

// TrustClaudeFolder is the best-effort, logged form of claudecfg.TrustFolder.
func TrustClaudeFolder(dir string) {
	if err := claudecfg.TrustFolder(nil, dir); err != nil {
		logger.Warn(context.Background(), "claude trust: mark folder trusted", "dir", dir, "err", err)
	}
}
