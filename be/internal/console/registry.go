package console

import (
	"fmt"
	"sort"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/tools_builtin"
)

// reusedBuiltins lists the session-independent builtin tools reused as-is in
// the console profile. Every other builtin (agent_*, findings_*,
// emit_findings, workflow_skip, chain_next_*, run_subworkflow/
// get_subworkflow, dynamic_workflow/revise_plan/approve_plan, consult,
// read_document, artifact_add and the builtin artifact_list/artifact_get) is
// session-bound (needs WorkflowInstanceID, session-scoped findings, or
// lifecycle semantics a console session has none of) and is deliberately
// excluded — see CLAUDE.md.
func reusedBuiltins() []string {
	return []string{
		"project_findings_add",
		"project_findings_add_bulk",
		"project_findings_append",
		"project_findings_append_bulk",
		"project_findings_get",
		"project_findings_delete",
		"workflow_continue",
		"workflow_fail",
		"ticket_create",
		"ticket_update",
		"ticket_add_dependency",
		"web_search",
		"web_fetch",
	}
}

// BuildRegistry composes the console tool profile: the allowlisted
// session-independent builtins plus the console-only handlers. Errors when an
// allowlisted name is missing from tools_builtin.Builtins() — a rename guard,
// not a runtime condition.
func BuildRegistry(d Deps) (apirun.Registry, error) {
	builtins := tools_builtin.Builtins()
	reg := make(apirun.Registry)
	for _, name := range reusedBuiltins() {
		h, ok := builtins[name]
		if !ok {
			return nil, fmt.Errorf("console: allowlisted builtin %q not found in tools_builtin.Builtins()", name)
		}
		reg[name] = h
	}

	reg["workflow_run"] = workflowRunHandler{d: d}
	reg["workflow_stop"] = workflowStopHandler{d: d}
	reg["workflow_retry_failed"] = workflowRetryFailedHandler{d: d}
	reg["workflow_get"] = workflowGetHandler{d: d}
	reg["workflow_list"] = workflowListHandler{d: d}
	reg["project_list"] = projectListHandler{d: d}
	reg["project_status"] = projectStatusHandler{d: d}
	reg["ticket_list"] = ticketListHandler{d: d}
	reg["ticket_get"] = ticketGetHandler{d: d}
	reg["ticket_current"] = ticketCurrentHandler{d: d}
	reg["artifact_list"] = artifactListHandler{d: d}
	reg["artifact_get"] = artifactGetHandler{d: d}

	return reg, nil
}

// Specs returns the catalogue: every handler's Spec(), sorted by name.
func Specs(reg apirun.Registry) []provider.ToolSpec {
	specs := make([]provider.ToolSpec, 0, len(reg))
	for _, h := range reg {
		specs = append(specs, h.Spec())
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs
}
