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
// emit_findings, workflow_skip, chain_next_*, run_subworkflow, read_document,
// artifact_add and the builtin artifact_list/artifact_get) is session-bound
// (needs WorkflowInstanceID, session-scoped findings, or lifecycle semantics
// a console session has none of) and is deliberately excluded — see
// CLAUDE.md. dynamic_workflow/get_subworkflow/revise_plan/approve_plan/
// consult are the exception: they are console-only reimplementations
// (tools_dynamic.go, tools_plan.go, tools_consult.go), not reused from this
// map, because they route through Deps (Consultant/Orch) or a project guard
// instead of the session-bound WorkflowInstanceID the builtin handlers key
// off. delegate/get_delegation/merge_delegation reuse the builtins —
// NewToolEnv wires env.Delegator from Deps.Delegator, and the builtin
// handlers only key off that plus session/project identity, so console
// callers get the same wait_sec defaults and poll hints as api-mode agents.
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
		"delegate",
		"get_delegation",
		"merge_delegation",
	}
}

// BuildRegistry composes the console tool profile: the allowlisted
// session-independent builtins plus the console-only handlers, then filters
// to catalogue when it is non-empty (a Profile's tool allowlist — nil/empty
// keeps every tool, today's pre-profile behavior used by the console-tools
// endpoint, mcp-external, and any chat with no profile). Errors when an
// allowlisted name is missing from tools_builtin.Builtins() — a rename guard,
// not a runtime condition — or when catalogue names a tool this registry
// does not compose.
func BuildRegistry(d Deps, catalogue []string) (apirun.Registry, error) {
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
	reg["workflow_wait"] = workflowWaitHandler{d: d}
	reg["workflow_list"] = workflowListHandler{d: d}
	reg["project_list"] = projectListHandler{d: d}
	reg["project_status"] = projectStatusHandler{d: d}
	reg["ticket_list"] = ticketListHandler{d: d}
	reg["ticket_get"] = ticketGetHandler{d: d}
	reg["ticket_current"] = ticketCurrentHandler{d: d}
	reg["artifact_list"] = artifactListHandler{d: d}
	reg["artifact_get"] = artifactGetHandler{d: d}
	reg["dynamic_workflow"] = dynamicWorkflowHandler{d: d}
	reg["get_subworkflow"] = getSubworkflowHandler{d: d}
	reg["revise_plan"] = revisePlanHandler{d: d}
	reg["approve_plan"] = approvePlanHandler{d: d}
	reg["consult"] = consultHandler{d: d}

	if len(catalogue) == 0 {
		return apirun.WrapToolAudit(reg), nil
	}
	filtered := make(apirun.Registry, len(catalogue))
	for _, name := range catalogue {
		h, ok := reg[name]
		if !ok {
			return nil, fmt.Errorf("console: profile catalogue names unknown tool %q", name)
		}
		filtered[name] = h
	}
	return apirun.WrapToolAudit(filtered), nil
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
