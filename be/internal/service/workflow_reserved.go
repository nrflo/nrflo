package service

import "strings"

// GlobalProjectID is the reserved storage namespace that holds global
// (project-agnostic) workflow definitions. It is NOT a user project: it is
// hidden from every project listing, and the authoritative "this is global"
// signal is the workflows.is_global column, not this string. Workflow
// resolution falls back to this namespace when a workflow is not defined under
// the selected project; execution still happens under the real selected project.
const GlobalProjectID = "__global__"

// DeepResearchWorkflow is the reserved name of the built-in deep-research
// workflow that the web_deep_research tool runs as a synchronous sub-workflow.
const DeepResearchWorkflow = "deep-research"

// IsReservedWorkflowName returns true for internal system workflow names like
// __spec_import__. Reserved workflows are excluded from the workflow listing.
func IsReservedWorkflowName(name string) bool {
	return strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__")
}
