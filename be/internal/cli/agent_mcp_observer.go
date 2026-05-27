package cli

import (
	"encoding/json"
	"strings"
)

// Observer-mode MCP tools. Observer agents (NRF_OBSERVER=1) are spawned outside
// the orchestrator, so the bridge cannot proxy through tools.call. Instead each
// observer tool maps to an observer.* socket method dispatched directly by the
// bridge (session_id injected from NRF_SESSION_ID; the socket handler enforces
// kind=observer + scope). The tool set mirrors the former `nrflo observer …`
// CLI commands. Mutate methods also re-check experimental_observer_enabled.
var observerMethodList = []struct{ method, desc string }{
	{"workflow.show", "Get the attached workflow definition. Input: {project_id?, workflow_id?}"},
	{"workflow.runs", "List workflow instances for the attached workflow. Input: {project_id?, workflow_id?}"},
	{"workflow.findings", "Get findings for the attached workflow instance. Input: {instance_id?}"},
	{"workflow.logs", "Get agent messages for the most recent (or given) session. Input: {target_session_id?, limit?, offset?}"},
	{"workflow.trigger", "Start a workflow run (mutate). Input: {ticket_id?, instructions?, scope_type?}"},
	{"workflow.retry_failed", "Retry a failed workflow from its failed layer (mutate). Input: {target_session_id?}"},
	{"workflow.def.update", "Update the workflow definition (mutate). Input: WorkflowDefUpdateRequest fields"},
	{"project.workflows", "List workflow definitions for a project. Input: {project_id?}"},
	{"project.runs", "List project-scoped workflow instances. Input: {project_id?}"},
	{"project.findings", "Get project findings. Input: {project_id?}"},
	{"project.env.list", "List project env vars. Input: {project_id?}"},
	{"project.env.set", "Upsert a project env var (mutate). Input: {project_id?, name, value}"},
	{"project.env.unset", "Delete a project env var (mutate). Input: {project_id?, name}"},
	{"project.workflow.create", "Create a workflow definition (mutate). Input: {project_id?, ...WorkflowDefCreateRequest}"},
	{"project.workflow.delete", "Delete a workflow definition (mutate). Input: {project_id?, workflow_id}"},
	{"global.projects", "List all projects. Input: {}"},
	{"global.recent_sessions", "List recent agent sessions. Input: {project_id?, limit?}"},
	{"global.health", "DB ping + observer feature-flag status. Input: {}"},
	{"global.project.create", "Create a project (mutate). Input: {project_id, name?, root_path?, default_branch?}"},
	{"global.project.delete", "Delete a project (mutate). Input: {project_id}"},
}

// observerToolName maps a dotted observer action to its mcp tool name
// (e.g. "workflow.def.update" -> "observer_workflow_def_update").
func observerToolName(method string) string {
	return "observer_" + strings.ReplaceAll(method, ".", "_")
}

// observerToolSpecs returns the MCP tool specs for observer mode (tools/list).
func observerToolSpecs() []json.RawMessage {
	specs := make([]json.RawMessage, 0, len(observerMethodList))
	for _, m := range observerMethodList {
		b, _ := json.Marshal(map[string]interface{}{
			"name":        observerToolName(m.method),
			"description": m.desc,
			"inputSchema": map[string]interface{}{"type": "object", "additionalProperties": true},
		})
		specs = append(specs, b)
	}
	return specs
}

// observerSocketMethod maps an mcp tool name back to its observer.* socket method.
func observerSocketMethod(toolName string) (string, bool) {
	for _, m := range observerMethodList {
		if observerToolName(m.method) == toolName {
			return "observer." + m.method, true
		}
	}
	return "", false
}
