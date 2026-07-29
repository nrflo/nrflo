package service

import "strings"

// GlobalProjectID is the reserved storage namespace that holds global
// (project-agnostic) workflow definitions. It is NOT a user project: it is
// hidden from every project listing, and the authoritative "this is global"
// signal is the workflows.is_global column, not this string. Workflow
// resolution falls back to this namespace when a workflow is not defined under
// the selected project; execution still happens under the real selected project.
const GlobalProjectID = "__global__"

// IsHiddenWorkflowName returns true for internal/system workflow and instance
// names — any leading underscore, mirroring the transientAgentTypeExclusion
// rule for agent_type/phase (workflow_response.go). Covers __spec_import__,
// _delegate_host, and future hidden defs. Hidden workflows are excluded from
// workflow-def listings and instance listings.
func IsHiddenWorkflowName(name string) bool {
	return strings.HasPrefix(name, "_")
}
