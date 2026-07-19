package service

import "be/internal/db"

// Delegate builtin guard config keys (config KV: project override > global >
// default) and their defaults. Cloned from the sub-workflow guard pattern
// (subworkflow.go) so delegate reuses the identical project>global>default
// resolution via SubworkflowCap.
const (
	DelegateMaxFanoutKey = "delegate_max_fanout"
	DelegateMaxDepthKey  = "delegate_max_depth"

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
