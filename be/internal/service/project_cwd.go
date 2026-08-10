package service

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// pathCaseInsensitive folds case in path comparison on darwin, where the
// default APFS is case-insensitive: a shell cwd typed in the wrong case
// (PWD=/…/kdre for on-disk /…/KDRE) must still match the registered root.
var pathCaseInsensitive = runtime.GOOS == "darwin"

func foldPathCase(p string) string {
	if pathCaseInsensitive {
		return strings.ToLower(p)
	}
	return p
}

// canonProjectPath normalizes a filesystem path for comparison: absolute,
// symlink-resolved (best effort), cleaned. Symlink resolution matters on macOS
// where temp dirs live under /var → /private/var. Mirrors the console bridge's
// client-side canonPath (cli/agent_mcp_external_cwd.go) — the HTTP-path twin of
// this DB-backed resolver.
func canonProjectPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	a, err := filepath.Abs(p)
	if err != nil {
		a = p
	}
	if r, err := filepath.EvalSymlinks(a); err == nil {
		a = r
	}
	return filepath.Clean(a)
}

// ResolveByCwd returns the id of the (non-hidden) project whose root_path equals
// cwd or is the longest ancestor of it, matched at a path-segment boundary so
// /a/foobar does not match root /a/foo. Empty and root ("/") project paths are
// skipped so they can't become a catch-all. Returns "" when cwd is empty or no
// project matches — never an error for a bad cwd (the prefix check fails
// closed). A List() failure surfaces as an error so the caller can fall back.
func (s *ProjectService) ResolveByCwd(cwd string) (string, error) {
	cwd = foldPathCase(canonProjectPath(cwd))
	if cwd == "" {
		return "", nil
	}
	projects, err := s.List()
	if err != nil {
		return "", err
	}
	best, bestLen := "", -1
	for _, p := range projects {
		if !p.RootPath.Valid {
			continue
		}
		root := foldPathCase(canonProjectPath(p.RootPath.String))
		if root == "" || root == string(os.PathSeparator) {
			continue
		}
		if cwd == root || strings.HasPrefix(cwd, root+string(os.PathSeparator)) {
			if len(root) > bestLen {
				best, bestLen = p.ID, len(root)
			}
		}
	}
	return best, nil
}
