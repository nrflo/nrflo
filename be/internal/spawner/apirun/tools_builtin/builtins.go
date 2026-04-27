// Package tools_builtin houses the in-process Go tool handlers wired into
// API-mode agents (findings, project_findings, agent_*, workflow_skip).
//
// The handlers call the same services the socket handler calls for CLI
// agents and use service.BroadcastFromCtx to emit identical WS events, so
// API and CLI agents look indistinguishable to the rest of the system.
package tools_builtin

import "be/internal/spawner/apirun"

// Builtins returns the canonical map of builtin tool name -> handler.
// Spawner code intersects this with the agent definition's tools CSV via
// apirun.ResolveRegistry to build the per-agent registry.
func Builtins() map[string]apirun.ToolHandler {
	return map[string]apirun.ToolHandler{
		"findings_add":         findingsAddHandler{},
		"findings_add_bulk":    findingsAddBulkHandler{},
		"findings_append":      findingsAppendHandler{},
		"findings_append_bulk": findingsAppendBulkHandler{},
		"findings_get":         findingsGetHandler{},
		"findings_delete":      findingsDeleteHandler{},

		"project_findings_add":         projectFindingsAddHandler{},
		"project_findings_add_bulk":    projectFindingsAddBulkHandler{},
		"project_findings_append":      projectFindingsAppendHandler{},
		"project_findings_append_bulk": projectFindingsAppendBulkHandler{},
		"project_findings_get":         projectFindingsGetHandler{},
		"project_findings_delete":      projectFindingsDeleteHandler{},

		"agent_fail":           agentFailHandler{},
		"agent_continue":       agentContinueHandler{},
		"agent_callback":       agentCallbackHandler{},
		"agent_context_update": agentContextUpdateHandler{},

		"workflow_skip": workflowSkipHandler{},
	}
}
