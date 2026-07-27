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
		Description: "Start the bundled, plan-driven `dynamic` workflow as a top-level run for this project (no ticket): a planner drafts a multi-agent manifest from your instructions, then parks for you to approve. Returns {instance_id, status} immediately — status is normally \"planning\" while the planner works, so this call is never the end of the flow. Then: workflow_wait(instance_id) to block until it transitions (the plan statuses are not terminal, so watch state.status, not just terminal), get_subworkflow(instance_id) to read the draft, then revise_plan until it is right and approve_plan to run it. It parks at waiting_input when the planner raised questions and at waiting_approval when it did not; both are answered/approved through the same two tools. Read the final output from the workflow_final_result finding once status is completed.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"instructions":{"type":"string","description":"Goal for the planner to turn into a multi-agent plan. Say what the workers should cost: nodes bind templates, and cheap-tier templates should carry the bulk of the work — premium ones (opus/fable) are capped at dynwf_max_premium_workers (default 2), so reserve them for genuine final adjudication."}
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
