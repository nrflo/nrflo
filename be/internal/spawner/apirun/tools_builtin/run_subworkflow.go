package tools_builtin

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// subworkflowMaxWaitSec caps the optional bounded wait on both tools; it stays
// under the MCP bridge's socket read deadline so cli/codex callers never time
// out client-side while the server keeps running.
const subworkflowMaxWaitSec = 240

// subworkflowPollInterval is the terminal-status poll cadence during a bounded
// wait. A var so tests can shrink it.
var subworkflowPollInterval = 3 * time.Second

// runSubworkflowHandler implements run_subworkflow: start a callable workflow
// as a detached child run and return its instance id (async-with-poll; an
// optional bounded wait returns the result inline when the child is fast).
type runSubworkflowHandler struct{}

func (runSubworkflowHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "run_subworkflow",
		Description: "Start a callable multi-agent workflow (e.g. deep-research) as a sub-workflow with the given instructions. Returns {instance_id, status} immediately; poll with get_subworkflow. Set wait_sec to also wait inline up to that many seconds for the result.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
 "workflow":{"type":"string","description":"Name of a workflow flagged callable_as_subworkflow"},
 "instructions":{"type":"string","description":"Instructions / question for the sub-workflow"},
 "result_key":{"type":"string","description":"Finding key holding the result (default workflow_final_result; deep-research emits 'report')"},
 "wait_sec":{"type":"integer","description":"Optionally block up to this many seconds (max 240) waiting for completion"}
},
"required":["workflow","instructions"],
"additionalProperties":false
}`),
	}
}

func (runSubworkflowHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Workflow     string `json:"workflow"`
		Instructions string `json:"instructions"`
		ResultKey    string `json:"result_key"`
		WaitSec      int    `json:"wait_sec"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if strings.TrimSpace(args.Workflow) == "" || strings.TrimSpace(args.Instructions) == "" {
		return "workflow and instructions are required", true, nil
	}
	if env.Subworkflows == nil {
		return missingService("subworkflows")
	}
	if env.Pool != nil && !service.SubworkflowToolsEnabled(env.Pool, env.ProjectID) {
		return "sub-workflow tools are disabled (subworkflow_tools_enabled=false)", true, nil
	}

	instanceID, err := env.Subworkflows.StartSubworkflow(ctx, env.WorkflowInstanceID, env.ProjectID, args.Workflow, args.Instructions)
	if err != nil {
		return err.Error(), true, nil
	}
	if args.WaitSec <= 0 {
		return subworkflowJSON(instanceID, "running", nil, ""), false, nil
	}
	return pollSubworkflow(ctx, env, instanceID, args.ResultKey, args.WaitSec)
}

// pollSubworkflow waits up to waitSec (capped) for the child to reach a terminal
// status, heartbeating the caller so stall detection stays quiet, and returns
// the current status either way (never an error on timeout — the caller can
// keep polling with get_subworkflow).
func pollSubworkflow(ctx context.Context, env apirun.ToolEnv, instanceID, resultKey string, waitSec int) (string, bool, error) {
	if waitSec > subworkflowMaxWaitSec {
		waitSec = subworkflowMaxWaitSec
	}
	deadline := time.NewTimer(time.Duration(waitSec) * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(subworkflowPollInterval)
	defer ticker.Stop()
	heartbeatEvery := 0

	for {
		status, result, failureReason, err := env.Subworkflows.GetSubworkflow(ctx, env.WorkflowInstanceID, env.ProjectID, instanceID, resultKey)
		if err != nil {
			return err.Error(), true, nil
		}
		if status != "running" {
			return subworkflowJSON(instanceID, status, result, failureReason), status == "failed", nil
		}
		select {
		case <-ctx.Done():
			return subworkflowJSON(instanceID, "running", nil, ""), false, nil
		case <-deadline.C:
			return subworkflowJSON(instanceID, "running", nil, ""), false, nil
		case <-ticker.C:
			heartbeatEvery++
			if env.Heartbeat != nil && heartbeatEvery%10 == 0 { // ~every 30s
				env.Heartbeat()
			}
		}
	}
}

// subworkflowJSON renders the common tool result payload for both tools.
func subworkflowJSON(instanceID, status string, result json.RawMessage, failureReason string) string {
	out := map[string]interface{}{"instance_id": instanceID, "status": status}
	if len(result) > 0 {
		out["result"] = result
	} else if status == "completed" {
		out["note"] = "completed but the result finding is empty; check result_key"
	}
	if failureReason != "" {
		out["failure_reason"] = failureReason
	}
	b, _ := json.Marshal(out)
	return string(b)
}
