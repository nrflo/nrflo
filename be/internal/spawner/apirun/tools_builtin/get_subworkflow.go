package tools_builtin

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// getSubworkflowHandler implements get_subworkflow: poll a child run started by
// run_subworkflow or dynamic_workflow. Terminal statuses include the result
// finding (or failure reason); the four plan-boundary statuses
// (planning/plan_ready/waiting_input/waiting_approval) include the current
// plan draft (plan/revision/questions) so the caller can act via
// revise_plan/approve_plan. An optional bounded wait long-polls instead of
// returning immediately.
type getSubworkflowHandler struct{}

func (getSubworkflowHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "get_subworkflow",
		Description: "Poll a sub-workflow started with run_subworkflow or dynamic_workflow. Returns {instance_id, status} and, when completed/failed, the result finding or failure reason; when parked at the plan boundary (planning/plan_ready/waiting_input/waiting_approval), the current plan draft {plan, revision, questions} — drive it further with revise_plan/approve_plan. Set wait_sec to long-poll up to that many seconds (max 240); it returns as soon as the child reaches a plan-boundary or terminal status.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
 "instance_id":{"type":"string","description":"Instance id returned by run_subworkflow or dynamic_workflow"},
 "result_key":{"type":"string","description":"Finding key holding the result (default workflow_final_result; deep-research emits 'report')"},
 "wait_sec":{"type":"integer","description":"Optionally block up to this many seconds (max 240) waiting for completion or a plan-boundary status"}
},
"required":["instance_id"],
"additionalProperties":false
}`),
	}
}

func (getSubworkflowHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		InstanceID string `json:"instance_id"`
		ResultKey  string `json:"result_key"`
		WaitSec    int    `json:"wait_sec"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if strings.TrimSpace(args.InstanceID) == "" {
		return "instance_id is required", true, nil
	}
	if env.Subworkflows == nil {
		return missingService("subworkflows")
	}
	if env.Pool != nil && !service.SubworkflowToolsEnabled(env.Pool, env.ProjectID) {
		return "sub-workflow tools are disabled (subworkflow_tools_enabled=false)", true, nil
	}

	if args.WaitSec > 0 {
		return pollSubworkflow(ctx, env, args.InstanceID, args.ResultKey, args.WaitSec)
	}
	state, err := env.Subworkflows.GetSubworkflow(ctx, env.WorkflowInstanceID, env.ProjectID, args.InstanceID, args.ResultKey)
	if err != nil {
		return err.Error(), true, nil
	}
	return subworkflowJSON(args.InstanceID, state), state.Status == "failed", nil
}
