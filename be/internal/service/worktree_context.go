package service

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// seedAgentContext seeds a fresh worktree with agent-context files that exist
// in the project root but are untracked/gitignored, so `git worktree add`
// does not materialize them (many projects gitignore these):
//
//   - CLAUDE.md / AGENTS.md at any depth: copied (snapshot per worktree).
//   - .claude directories at any depth: symlinked back to the main checkout,
//     so skills/settings stay in sync for the run's whole lifetime.
//
// Existing paths in the worktree (tracked content the checkout produced) are
// never touched. Best-effort: failures log and never abort worktree setup.
func seedAgentContext(projectRoot, worktreePath string) {
	out, err := runGit(projectRoot, "ls-files", "-o", "-z", "--",
		":(glob)**/CLAUDE.md", ":(glob)**/AGENTS.md", ":(glob)**/.claude/**")
	if err != nil {
		log.Printf("worktree agent-context: list failed for %s: %v", projectRoot, err)
		return
	}
	claudeDirs := map[string]bool{}
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" {
			continue
		}
		if dir, ok := claudeDirOf(rel); ok {
			claudeDirs[dir] = true
			continue
		}
		if err := copyContextFile(projectRoot, worktreePath, rel); err != nil {
			log.Printf("worktree agent-context: copy %s failed: %v", rel, err)
		}
	}
	for dir := range claudeDirs {
		if err := linkContextDir(projectRoot, worktreePath, dir); err != nil {
			log.Printf("worktree agent-context: link %s failed: %v", dir, err)
		}
	}
	warnVisibleSeeds(worktreePath)
}

// warnVisibleSeeds flags seeded paths git can see in the worktree (e.g. a
// `.claude/` dir-only ignore pattern does not match the seeded symlink):
// an agent's `git add -A` would commit them. Seeding still proceeds — the
// context files matter more — but the operator should fix the pattern.
func warnVisibleSeeds(worktreePath string) {
	out, err := runGit(worktreePath, "status", "--porcelain")
	if err != nil || strings.TrimSpace(out) == "" {
		return
	}
	log.Printf("worktree agent-context: seeded paths visible to git in %s (adjust the project's ignore patterns, e.g. use %q not %q):\n%s",
		worktreePath, ".claude", ".claude/", strings.TrimSpace(out))
}

// claudeDirOf returns the .claude directory (repo-relative) containing rel,
// when rel lies inside one.
func claudeDirOf(rel string) (string, bool) {
	if strings.HasPrefix(rel, ".claude/") {
		return ".claude", true
	}
	if i := strings.Index(rel, "/.claude/"); i >= 0 {
		return rel[:i+len("/.claude")], true
	}
	return "", false
}

// linkContextDir symlinks worktreePath/rel -> projectRoot/rel unless the
// worktree already has something there (e.g. tracked .claude content).
func linkContextDir(projectRoot, worktreePath, rel string) error {
	dst := filepath.Join(worktreePath, rel)
	if _, err := os.Lstat(dst); err == nil {
		return nil
	}
	src, err := filepath.Abs(filepath.Join(projectRoot, rel))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.Symlink(src, dst)
}

// copyContextFile copies projectRoot/rel to worktreePath/rel, creating parent
// directories and preserving the file mode. Existing destinations are left
// alone (tracked checkout content wins).
func copyContextFile(projectRoot, worktreePath, rel string) error {
	src := filepath.Join(projectRoot, rel)
	info, err := os.Stat(src)
	if err != nil || info.IsDir() {
		return err // vanished or unexpected dir entry; ls-files lists files only
	}
	dst := filepath.Join(worktreePath, rel)
	if _, err := os.Lstat(dst); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
