package tools_builtin

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// dynamicWorkflowHandler implements dynamic_workflow: start the bundled
// plan-driven `dynamic` workflow (service.DynamicWorkflow) as a detached child
// sharing run_subworkflow's guards/caps. Async-with-poll, same as
// run_subworkflow — the child parks at waiting_approval (mode="approve",
// default) or auto-approves and runs to completion unattended
// (mode="auto", gated by dynamic_workflow_auto_enabled).
type dynamicWorkflowHandler struct{}

func (dynamicWorkflowHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "dynamic_workflow",
		Description: "Start the bundled, plan-driven `dynamic` workflow as a sub-workflow: a planner drafts a multi-agent manifest from your instructions, then (mode=\"approve\", default) parks at waiting_approval for you to drive via get_subworkflow/revise_plan/approve_plan, or (mode=\"auto\", if enabled) auto-approves and runs to completion unattended. Returns {instance_id, status} immediately; poll with get_subworkflow. Set wait_sec to also wait inline up to that many seconds — it returns as soon as the draft leaves planning.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
 "instructions":{"type":"string","description":"Goal / instructions for the planner to turn into a multi-agent plan."},
 "mode":{"type":"string","enum":["approve","auto"],"description":"\"approve\" (default) parks at waiting_approval for you to drive; \"auto\" auto-approves and materializes without suspending (requires dynamic_workflow_auto_enabled)"},
 "wait_sec":{"type":"integer","description":"Optionally block up to this many seconds (max 240) waiting for the draft (or completion)"}
},
"required":["instructions"],
"additionalProperties":false
}`),
	}
}

func (dynamicWorkflowHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Instructions string `json:"instructions"`
		Mode         string `json:"mode"`
		WaitSec      int    `json:"wait_sec"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if strings.TrimSpace(args.Instructions) == "" {
		return "instructions is required", true, nil
	}
	if env.Subworkflows == nil {
		return missingService("subworkflows")
	}
	if env.Pool != nil && !service.SubworkflowToolsEnabled(env.Pool, env.ProjectID) {
		return "sub-workflow tools are disabled (subworkflow_tools_enabled=false)", true, nil
	}

	instanceID, err := env.Subworkflows.StartDynamicWorkflow(ctx, env.WorkflowInstanceID, env.ProjectID, args.Instructions, args.Mode)
	if err != nil {
		return err.Error(), true, nil
	}
	if args.WaitSec <= 0 {
		return subworkflowJSON(instanceID, apirun.SubworkflowState{Status: "planning"}), false, nil
	}
	return pollSubworkflow(ctx, env, instanceID, "", args.WaitSec)
}
