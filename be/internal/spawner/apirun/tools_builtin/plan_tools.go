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
		Description: "Append a new revision to a sub-workflow's plan (started via dynamic_workflow or run_subworkflow), at the plan boundary. Two modes, and plan wins: set plan to store your own edited manifest (feedback and answers are then ignored), or omit plan to re-run the planner with feedback+answers. Both are synchronous: a planner re-run blocks until the child settles, and the response IS the new revision {revision, manifest, author} — pin that number into the next call rather than re-polling. The revision increments by exactly 1. revision must match the child's current revision or the call is rejected as stale (the error names the current one). Only a draft plan can be revised — once approved, revise_plan is rejected.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
 "instance_id":{"type":"string","description":"Instance id returned by dynamic_workflow / run_subworkflow"},
 "revision":{"type":"integer","description":"Must match the current plan revision from get_subworkflow; pass 0 while status is planning and no draft exists yet, which seeds the first revision (with plan, your manifest becomes it and the planner never runs; with feedback, the planner drafts from it)"},
 "plan":{"type":"object","description":"Full manifest, stored as-is after validation. Exact v1 shape, no other fields accepted at any level: {\"version\":1,\"goal\":\"...\",\"layers\":[{\"layer\":0,\"policy\":\"any\",\"nodes\":[{\"id\":\"map\",\"template\":\"<an id from get_subworkflow.templates>\",\"instructions\":\"...\"}]}]}. STRUCTURE: layers must be dense and 0-indexed (a layer's layer number equals its position), run in ascending order, and nodes inside one layer run concurrently; the LAST layer must hold exactly one node — it carries the run's result. Node ids are unique across the whole plan and match ^[a-z0-9][a-z0-9_-]{0,63}$. The one other field the manifest may carry is a top-level questions array ([{\"id\",\"question\"}], the planner's open questions) — omit it when authoring your own plan. CAPS: at most 4 layers, 25 nodes total, 4000 bytes per node's instructions and 10 questions (project-config plan_max_layers/plan_max_nodes/plan_max_instruction_bytes/plan_max_questions); a manifest over a cap is rejected naming the limit. DATA FLOW: a node sees only its own instructions, so to consume an earlier node's output put #{NODE_FINDINGS:<node-id>} (or #{NODE_FINDINGS:<node-id>:key1,key2}) inside its instructions; the target must sit in a strictly earlier layer. Interpolation is failure-tolerant, never an error: a referenced node that recorded nothing renders a no-findings placeholder, a failed node's pre-failure findings stay readable, and an unknown node id renders empty. POLICY: the layer's pass rule — any (1 node must pass), all, quorum:N, percent:P (ceil of P% of the layer's nodes) — it gates whether the run advances past the layer and never affects which findings a later node can read. TIER: a node has NO model/effort/tools field, its template decides those, so bind a cheap-tier template to keep a worker cheap; premium-tier nodes are capped at the premium_cap get_subworkflow returns (dynwf_max_premium_workers, default 2) and over the cap this call is rejected, naming the offending nodes. FINDING KEYS: each template's description names the finding key its node emits to, which is the key to use in the #{NODE_FINDINGS:<node-id>:<key>} form. FAILURE: a layer whose policy is not met fails the whole run — status failed, with failure_reason naming the layer, its policy and the passed/required counts. RESULT: the run's final value is read from the workflow_final_result finding key literally, no aliasing — keep the last node bound to a result-carrying template (its description says it emits workflow_final_result); if it emits another key, read that back via get_subworkflow's result_key."},
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
		Description: "Approve a sub-workflow's plan at the given revision: materializes the manifest into agent nodes and resumes the child if it was parked at the plan boundary. Open questions never block approval — approve when the plan is good enough. revision must match the child's current revision (see get_subworkflow) or the call is rejected as stale. The manifest is re-validated here, so approve can fail at the correct revision too: a template's model may have been disabled since the draft, or the plan may exceed dynwf_max_premium_workers — fix it with revise_plan and approve the new revision. Re-approving an already-approved revision is idempotent and retries the child's resume, which is the recovery path when the plan was approved but the run did not restart.",
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
