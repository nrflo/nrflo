package console

import (
	"context"
	"encoding/json"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// workflowGetHandler implements workflow_get: the same v4 state map
// WorkflowService.GetStatusByInstance returns for GET /api/v1/tickets/{id}/workflow
// and GET /api/v1/projects/{id}/workflow.
type workflowGetHandler struct{ d Deps }

func (workflowGetHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "workflow_get",
		Description: "Get the v4 status state of a workflow instance.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{"instance_id":{"type":"string"}},
"required":["instance_id"],
"additionalProperties":false
}`),
	}
}

func (h workflowGetHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		InstanceID string `json:"instance_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if args.InstanceID == "" {
		return "instance_id is required", true, nil
	}
	if h.d.WorkflowSvc == nil {
		return missingService("workflow")
	}
	wi, err := loadGuardedInstance(h.d, env.ProjectID, args.InstanceID)
	if err != nil {
		return err.Error(), true, nil
	}
	state, err := h.d.WorkflowSvc.GetStatusByInstance(wi)
	if err != nil {
		return err.Error(), true, nil
	}
	out, err := json.Marshal(state)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(out), false, nil
}

// workflowListHandler implements workflow_list: the project's + global
// workflow definitions, mirroring GET /api/v1/workflows.
type workflowListHandler struct{ d Deps }

func (workflowListHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "workflow_list",
		Description: "List the project's selectable workflow definitions (includes global definitions like deep-research).",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}
}

func (h workflowListHandler) Invoke(ctx context.Context, env apirun.ToolEnv, _ json.RawMessage) (string, bool, error) {
	if h.d.WorkflowSvc == nil {
		return missingService("workflow")
	}
	defs, err := h.d.WorkflowSvc.ListWorkflowDefs(env.ProjectID)
	if err != nil {
		return err.Error(), true, nil
	}
	out, err := json.Marshal(defs)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(out), false, nil
}
