package cli

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// pathCaseInsensitive folds case in path comparison on darwin, where the
// default APFS is case-insensitive: a shell cwd typed in the wrong case must
// still match the registered root. Mirrors service.foldPathCase
// (service/project_cwd.go), this resolver's server-side twin.
var pathCaseInsensitive = runtime.GOOS == "darwin"

func foldPathCase(p string) string {
	if pathCaseInsensitive {
		return strings.ToLower(p)
	}
	return p
}

// projRoot is the subset of a project needed to match a directory to a project.
type projRoot struct {
	ID       string `json:"id"`
	RootPath string `json:"root_path"`
}

// canonPath normalizes a filesystem path for comparison: absolute,
// symlink-resolved (best effort), cleaned. Symlink resolution matters on macOS
// where temp dirs live under /var → /private/var.
func canonPath(p string) string {
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

// matchProjectByCwd returns the id of the project whose root_path equals cwd or
// is the longest ancestor of cwd (matched at a path-segment boundary, so
// /a/foobar does not match root /a/foo). Empty and root ("/") project paths are
// skipped so they can't become a catch-all. Pure — no I/O.
func matchProjectByCwd(cwd string, projects []projRoot) string {
	cwd = foldPathCase(canonPath(cwd))
	if cwd == "" {
		return ""
	}
	best, bestLen := "", -1
	for _, p := range projects {
		root := foldPathCase(canonPath(p.RootPath))
		if root == "" || root == string(os.PathSeparator) {
			continue
		}
		if cwd == root || strings.HasPrefix(cwd, root+string(os.PathSeparator)) {
			if len(root) > bestLen {
				best, bestLen = p.ID, len(root)
			}
		}
	}
	return best
}

// listProjects fetches the (non-hidden) projects with their root_path. It uses
// the SERVICE token (never the console bearer, which may be the stale one being
// re-exchanged). X-Project is empty: the list endpoint is project-agnostic
// (protected, not projectAdmin), and a service token — global or project-scoped
// — may read it.
func (c *nrfloHTTPClient) listProjects(ctx context.Context) ([]projRoot, error) {
	var res struct {
		Projects []projRoot `json:"projects"`
	}
	if err := c.doAs(ctx, c.serviceToken, "", http.MethodGet, "/api/v1/projects", nil, &res); err != nil {
		return nil, err
	}
	return res.Projects, nil
}

// cwdProject resolves the proxy's working directory to a project id. The result
// is cached for the process lifetime, but ONLY on success — a failed lookup
// (Getwd error, or listProjects failing incl. a cancelled context) is NOT
// cached, so a transient first-call failure is retried on the next call rather
// than permanently disabling auto-detect. Best-effort: any failure degrades to
// "" so resolution falls through to NRFLO_PROJECT / the global home — a bad cwd
// never produces a wrong match (the prefix check fails closed). The cache is
// mutex-guarded (c.mu); the lock is never held across the HTTP call, so two
// concurrent first lookups may both run — the result is idempotent.
func (c *nrfloHTTPClient) cwdProject(ctx context.Context) string {
	c.mu.Lock()
	resolved, id := c.cwdResolved, c.cwdProjectID
	c.mu.Unlock()
	if resolved {
		return id
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "" // transient — do not cache; retry next call
	}
	projects, err := c.listProjects(ctx)
	if err != nil {
		return "" // transient (incl. ctx cancel) — do not cache; retry next call
	}
	id = matchProjectByCwd(cwd, projects) // cache success, incl. legit "no match"
	c.mu.Lock()
	c.cwdProjectID, c.cwdResolved = id, true
	c.mu.Unlock()
	return id
}
