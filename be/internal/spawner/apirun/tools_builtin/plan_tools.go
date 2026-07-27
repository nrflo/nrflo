package tools_builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/types"
)

// revisePlanHandler implements revise_plan: append a new plan revision to a
// child started by dynamic_workflow/run_subworkflow, either from a
// caller-edited manifest (plan) or planner feedback/answers — mirroring
// POST .../plan/revise. Revision-pinned: a stale revision or a non-draft head
// surfaces as isError=true, best-effort naming the child's current revision so
// the caller can retry.
type revisePlanHandler struct{}

func (revisePlanHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "revise_plan",
		Description: "Append a new revision to a sub-workflow's plan (started via dynamic_workflow or run_subworkflow), at the plan boundary. Two modes, and plan wins: set plan to store your own edited manifest (feedback and answers are then ignored), or omit plan to re-run the planner with feedback+answers. Either way the revision number increments by exactly 1, so re-read get_subworkflow before the next call. revision must match the child's current revision or the call is rejected as stale (the error names the current one). Only a draft plan can be revised — once approved, revise_plan is rejected.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
 "instance_id":{"type":"string","description":"Instance id returned by dynamic_workflow / run_subworkflow"},
 "revision":{"type":"integer","description":"Must match the child's current plan revision (0 if it has none yet)"},
 "plan":{"type":"object","description":"Full manifest, stored as-is after validation. Exact v1 shape, no other fields accepted at any level: {\"version\":1,\"goal\":\"...\",\"layers\":[{\"layer\":0,\"policy\":\"any|all|quorum:N|percent:P\",\"nodes\":[{\"id\":\"unique-id\",\"template\":\"<one of get_subworkflow.templates[].id>\",\"instructions\":\"...\"}]}],\"questions\":[]}. Layers run in ascending order, nodes within a layer run concurrently. A node has NO model/effort/tools field — its template decides those, so binding a cheap-tier template is how you keep a worker cheap. Premium-tier nodes are capped at dynwf_max_premium_workers (default 2); over the cap this call is rejected, naming the offending nodes."},
 "feedback":{"type":"string","description":"Steer a planner re-run in prose. Ignored when plan is set."},
 "answers":{"type":"array","description":"Answers to the open questions get_subworkflow returned. Ignored when plan is set.","items":{"type":"object","properties":{"question_id":{"type":"string"},"answer":{"type":"string"}}}}
},
"required":["instance_id","revision"],
"additionalProperties":false
}`),
	}
}

func (revisePlanHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		InstanceID string             `json:"instance_id"`
		Revision   int                `json:"revision"`
		Plan       json.RawMessage    `json:"plan"`
		Feedback   string             `json:"feedback"`
		Answers    []types.PlanAnswer `json:"answers"`
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

	req := types.PlanReviseRequest{Revision: args.Revision, Manifest: args.Plan, Feedback: args.Feedback, Answers: args.Answers}
	rev, err := env.Subworkflows.RevisePlan(ctx, env.WorkflowInstanceID, env.ProjectID, args.InstanceID, req)
	if err != nil {
		return planServiceErrorText(ctx, env, args.InstanceID, err), true, nil
	}
	b, _ := json.Marshal(rev)
	return string(b), false, nil
}

// approvePlanHandler implements approve_plan: approve+materialize a
// sub-workflow's plan at the given revision, mirroring POST .../plan/approve.
type approvePlanHandler struct{}

func (approvePlanHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "approve_plan",
		Description: "Approve a sub-workflow's plan at the given revision: materializes the manifest into agent nodes and resumes the child if it was parked at the plan boundary. Open questions never block approval — approve when the plan is good enough. revision must match the child's current revision (see get_subworkflow) or the call is rejected as stale. The manifest is re-validated here, so approve can fail at the correct revision too: a template's model may have been disabled since the draft, or the plan may exceed dynwf_max_premium_workers — fix it with revise_plan and approve the new revision.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
 "instance_id":{"type":"string","description":"Instance id returned by dynamic_workflow / run_subworkflow"},
 "revision":{"type":"integer","description":"Must match the child's current plan revision"}
},
"required":["instance_id","revision"],
"additionalProperties":false
}`),
	}
}

func (approvePlanHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		InstanceID string `json:"instance_id"`
		Revision   int    `json:"revision"`
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

	rev, err := env.Subworkflows.ApprovePlan(ctx, env.WorkflowInstanceID, env.ProjectID, args.InstanceID, args.Revision)
	if err != nil {
		return planServiceErrorText(ctx, env, args.InstanceID, err), true, nil
	}
	b, _ := json.Marshal(rev)
	return string(b), false, nil
}

// planServiceErrorText renders a plan-service error for a tool result. For the
// stale-revision/not-draft sentinels it best-effort fetches the child's
// current revision (via GetSubworkflow) so the caller can retry with the
// right value; any other error is returned as-is.
func planServiceErrorText(ctx context.Context, env apirun.ToolEnv, instanceID string, err error) string {
	if !errors.Is(err, service.ErrStalePlanRevision) && !errors.Is(err, service.ErrPlanNotDraft) {
		return err.Error()
	}
	if env.Subworkflows == nil {
		return err.Error()
	}
	state, gerr := env.Subworkflows.GetSubworkflow(ctx, env.WorkflowInstanceID, env.ProjectID, instanceID, "")
	if gerr != nil || state.Revision == 0 {
		return err.Error()
	}
	return fmt.Sprintf("%s (current revision: %d)", err.Error(), state.Revision)
}
