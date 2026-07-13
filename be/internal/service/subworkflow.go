package service

import (
	"strconv"
	"strings"

	"be/internal/db"
)

// Sub-workflow guard config keys (config KV: project override > global > default)
// and their defaults. Shared by the orchestrator runner (server-side enforcement)
// and the spawner registry (tool stripping at spawn time).
const (
	SubworkflowToolsEnabledKey   = "subworkflow_tools_enabled"
	SubworkflowMaxDepthKey       = "subworkflow_max_depth"
	SubworkflowMaxChildrenKey    = "subworkflow_max_children"
	SubworkflowMaxInvocationsKey = "subworkflow_max_invocations"

	DefaultSubworkflowMaxDepth       = 3  // max child subworkflow_depth (nesting); unrelated to launch_depth (next-on-success lineage)
	DefaultSubworkflowMaxChildren    = 6  // concurrent children across all runs
	DefaultSubworkflowMaxInvocations = 25 // starts charged per parent run
)

// SubworkflowCap reads an integer guard from the config KV (project override
// first, then global), falling back to def when unset or unparsable.
func SubworkflowCap(pool *db.Pool, projectID, key string, def int) int {
	raw, err := pool.GetProjectConfig(projectID, key)
	if err != nil || raw == "" {
		raw, err = pool.GetConfig(key)
		if err != nil || raw == "" {
			return def
		}
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return def
	}
	return n
}

// SubworkflowToolsEnabled reports whether the run_subworkflow / get_subworkflow
// tools are enabled (default true; set the key to "false" to disable — project
// override first, then global). Re-checked at every tool invocation.
func SubworkflowToolsEnabled(pool *db.Pool, projectID string) bool {
	raw, err := pool.GetProjectConfig(projectID, SubworkflowToolsEnabledKey)
	if err != nil || raw == "" {
		raw, err = pool.GetConfig(SubworkflowToolsEnabledKey)
		if err != nil || raw == "" {
			return true
		}
	}
	return !strings.EqualFold(strings.TrimSpace(raw), "false")
}
