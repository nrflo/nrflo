package service

import (
	"strings"

	"be/internal/db"
)

// Delegate builtin guard config keys (config KV: project override > global >
// default) and their defaults. Cloned from the sub-workflow guard pattern
// (subworkflow.go) so delegate reuses the identical project>global>default
// resolution via SubworkflowCap.
const (
	DelegateMaxFanoutKey = "delegate_max_fanout"
	DelegateMaxDepthKey  = "delegate_max_depth"

	// DelegateWorktreeIsolationKey is the operator-only escape hatch for
	// per-delegation worktree isolation (config KV: project override >
	// global > default true). Deliberately not a `delegate` tool parameter —
	// a model-visible toggle would be routinely set to bypass isolation.
	DelegateWorktreeIsolationKey = "delegate_worktree_isolation"

	DefaultDelegateMaxFanout = 20 // max fanout items per delegate call
	DefaultDelegateMaxDepth  = 2  // max delegate nesting (T0->T1->T2); tool stripped past this
)

// DelegateMaxFanout reads the fanout cap guard from the config KV.
func DelegateMaxFanout(pool *db.Pool, projectID string) int {
	return SubworkflowCap(pool, projectID, DelegateMaxFanoutKey, DefaultDelegateMaxFanout)
}

// DelegateMaxDepth reads the recursion depth cap guard from the config KV.
func DelegateMaxDepth(pool *db.Pool, projectID string) int {
	return SubworkflowCap(pool, projectID, DelegateMaxDepthKey, DefaultDelegateMaxDepth)
}

// DelegateWorktreeIsolation reports whether per-delegation worktree
// isolation is enabled (default true; project override first, then global).
// Clones SubworkflowToolsEnabled's shape.
func DelegateWorktreeIsolation(pool *db.Pool, projectID string) bool {
	raw, err := pool.GetProjectConfig(projectID, DelegateWorktreeIsolationKey)
	if err != nil || raw == "" {
		raw, err = pool.GetConfig(DelegateWorktreeIsolationKey)
		if err != nil || raw == "" {
			return true
		}
	}
	return !strings.EqualFold(strings.TrimSpace(raw), "false")
}
