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
// The exact set of paths actually seeded is recorded in the worktree's
// private gitdir (seededContextFile) so CommitAndCollect can unstage them —
// in a project that does not gitignore these files, a `git add -A` would
// otherwise sweep them into the delegation commit and the eventual merge.
func seedAgentContext(projectRoot, worktreePath string) {
	out, err := runGit(projectRoot, "ls-files", "-o", "-z", "--",
		":(glob)**/CLAUDE.md", ":(glob)**/AGENTS.md", ":(glob)**/.claude/**")
	if err != nil {
		log.Printf("worktree agent-context: list failed for %s: %v", projectRoot, err)
		return
	}
	seeded := []string{}
	claudeDirs := map[string]bool{}
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" {
			continue
		}
		if dir, ok := claudeDirOf(rel); ok {
			claudeDirs[dir] = true
			continue
		}
		if didSeed, err := copyContextFile(projectRoot, worktreePath, rel); err != nil {
			log.Printf("worktree agent-context: copy %s failed: %v", rel, err)
		} else if didSeed {
			seeded = append(seeded, rel)
		}
	}
	for dir := range claudeDirs {
		if didSeed, err := linkContextDir(projectRoot, worktreePath, dir); err != nil {
			log.Printf("worktree agent-context: link %s failed: %v", dir, err)
		} else if didSeed {
			seeded = append(seeded, dir)
		}
	}
	recordSeededContext(worktreePath, seeded)
	warnVisibleSeeds(worktreePath)
}

// seededContextFile is the filename (inside the worktree's private gitdir,
// which `git worktree remove` deletes with the worktree) recording the
// repo-relative paths seedAgentContext materialized, one per line.
const seededContextFile = "nrflo-seeded-context"

// recordSeededContext persists the seeded path list next to the worktree's
// git state. Best-effort: a failure logs and the commit path falls back to
// warnVisibleSeeds' operator warning.
func recordSeededContext(worktreePath string, seeded []string) {
	if len(seeded) == 0 {
		return
	}
	gitDir, err := runGit(worktreePath, "rev-parse", "--absolute-git-dir")
	if err != nil {
		log.Printf("worktree agent-context: resolve gitdir failed for %s: %v", worktreePath, err)
		return
	}
	path := filepath.Join(strings.TrimSpace(gitDir), seededContextFile)
	if err := os.WriteFile(path, []byte(strings.Join(seeded, "\n")+"\n"), 0o644); err != nil {
		log.Printf("worktree agent-context: record seeded paths failed: %v", err)
	}
}

// seededContextPaths reads back the recorded seed list for a worktree; empty
// (never an error) when nothing was recorded.
func seededContextPaths(worktreePath string) []string {
	gitDir, err := runGit(worktreePath, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(strings.TrimSpace(gitDir), seededContextFile))
	if err != nil {
		return nil
	}
	paths := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}
	return paths
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
// Reports whether it actually created the link — pre-existing destinations
// are tracked checkout content and must never be recorded as seeds.
func linkContextDir(projectRoot, worktreePath, rel string) (bool, error) {
	dst := filepath.Join(worktreePath, rel)
	if _, err := os.Lstat(dst); err == nil {
		return false, nil
	}
	src, err := filepath.Abs(filepath.Join(projectRoot, rel))
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	if err := os.Symlink(src, dst); err != nil {
		return false, err
	}
	return true, nil
}

// copyContextFile copies projectRoot/rel to worktreePath/rel, creating parent
// directories and preserving the file mode. Existing destinations are left
// alone (tracked checkout content wins). Reports whether it actually wrote
// the copy.
func copyContextFile(projectRoot, worktreePath, rel string) (bool, error) {
	src := filepath.Join(projectRoot, rel)
	info, err := os.Stat(src)
	if err != nil || info.IsDir() {
		return false, err // vanished or unexpected dir entry; ls-files lists files only
	}
	dst := filepath.Join(worktreePath, rel)
	if _, err := os.Lstat(dst); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	in, err := os.Open(src)
	if err != nil {
		return false, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return false, err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return false, err
	}
	return true, out.Close()
}
