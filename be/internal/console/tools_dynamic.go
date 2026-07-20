package console

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// dynamicWorkflowHandler implements the console-scoped dynamic_workflow:
// unlike the session-bound builtin (tools_builtin.dynamicWorkflowHandler,
// which starts a *child* run via apirun.SubworkflowRunner under the caller's
// own WorkflowInstanceID), a console session has none, so this starts a
// top-level project-scoped `dynamic` run via Deps.Orch.StartWorkflow — no
// parent instance needed. Always plan mode (StartWorkflow never sets
// PlanAutoApprove): the run suspends at waiting_approval for the caller to
// drive via get_subworkflow/revise_plan/approve_plan.
type dynamicWorkflowHandler struct{ d Deps }

func (dynamicWorkflowHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "dynamic_workflow",
		Description: "Start the bundled, plan-driven `dynamic` workflow as a top-level project-scoped run: a planner drafts a multi-agent manifest from your instructions, then parks at waiting_approval for you to drive via get_subworkflow/revise_plan/approve_plan. Returns {instance_id,status} immediately; poll with get_subworkflow.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"instructions":{"type":"string","description":"Goal / instructions for the planner to turn into a multi-agent plan. Default worker nodes to cheap tier; reserve premium (opus/fable) for genuine final-adjudication needs."}
},
"required":["instructions"],
"additionalProperties":false
}`),
	}
}

func (h dynamicWorkflowHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if strings.TrimSpace(args.Instructions) == "" {
		return "instructions is required", true, nil
	}
	if h.d.Orch == nil {
		return missingService("orchestrator")
	}

	instanceID, err := h.d.Orch.StartWorkflow(ctx, env.ProjectID, "", service.DynamicWorkflow, args.Instructions, "project")
	if err != nil {
		return err.Error(), true, nil
	}

	status := "planning"
	if wi, werr := loadGuardedInstance(h.d, env.ProjectID, instanceID); werr == nil {
		status = string(wi.Status)
	}
	out, err := json.Marshal(map[string]string{"instance_id": instanceID, "status": status})
	if err != nil {
		return err.Error(), true, nil
	}
	return string(out), false, nil
}
