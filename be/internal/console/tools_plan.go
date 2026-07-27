package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/types"
	"be/internal/ws"
)

// subworkflowStateFor builds the same apirun.SubworkflowState shape the
// session-bound get_subworkflow builtin returns (orchestrator.GetSubworkflow)
// for wi, whose caller has already been authorized via loadGuardedInstance
// (project match) instead of orchestrator.assertChildOwnership (parent
// match) — a console session has no parent workflow instance to own wi
// under.
func subworkflowStateFor(d Deps, wi *model.WorkflowInstance, resultKey string) apirun.SubworkflowState {
	findingRepo := repo.NewFindingRepo(d.Pool, d.Clock)
	switch wi.Status {
	case model.WorkflowInstanceCompleted, model.WorkflowInstanceProjectCompleted:
		key := resultKey
		if key == "" {
			key = "workflow_final_result"
		}
		val, _ := findingRepo.GetSessionFindingByKey(wi.ID, key)
		return apirun.SubworkflowState{Status: "completed", Result: val}
	case model.WorkflowInstanceFailed:
		reason := ""
		if own, ferr := findingRepo.GetOwn("workflow_instance", wi.ID); ferr == nil {
			if raw, ok := own["_failure_reason"]; ok {
				var fr struct {
					Reason string `json:"reason"`
				}
				if json.Unmarshal(raw, &fr) == nil {
					reason = fr.Reason
				}
			}
		}
		return apirun.SubworkflowState{Status: "failed", FailureReason: reason}
	case model.WorkflowInstanceWaiting:
		return apirun.SubworkflowState{Status: "waiting", FailureReason: "paused after a pause_after layer; requires human resume"}
	case model.WorkflowInstancePlanning, model.WorkflowInstancePlanReady, model.WorkflowInstanceWaitingInput, model.WorkflowInstanceWaitingApproval:
		state := apirun.SubworkflowState{Status: string(wi.Status)}
		if draft, derr := service.NewPlanService(d.Pool, d.Clock, d.Orch).GetDraft(wi.ID); derr == nil {
			if draft.Head != nil {
				state.Revision = draft.Head.LatestRevision
			}
			if draft.Manifest != nil {
				if raw, merr := json.Marshal(draft.Manifest); merr == nil {
					state.Plan = raw
				}
			}
			if len(draft.Questions) > 0 {
				if raw, qerr := json.Marshal(draft.Questions); qerr == nil {
					state.Questions = raw
				}
			}
		}
		state.Templates = service.PlanTemplateChoicesJSON(d.Pool, d.Clock, wi.ProjectID, wi.WorkflowID)
		state.PremiumCap = service.LoadDynwfMaxPremiumWorkers(d.Pool, wi.ProjectID)
		return state
	default:
		return apirun.SubworkflowState{Status: "running"}
	}
}

// getSubworkflowHandler implements the console-scoped get_subworkflow: polls
// any project-owned instance_id (not just one this session started) since a
// console session has no parent-run ownership chain to key off.
type getSubworkflowHandler struct{ d Deps }

func (getSubworkflowHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "get_subworkflow",
		Description: "Poll one workflow instance in this project; returns {status, ...}. status is one of: running; waiting (paused after a pause_after layer — resume with workflow_continue, NOT approve_plan); planning, waiting_input, waiting_approval (a plan-driven run parked at the plan boundary — planning means the planner is still drafting, waiting_input means the draft has open questions, waiting_approval means it is ready); completed (result = the run's workflow_final_result session finding, or the result_key you name); failed (failure_reason — for a plan-driven run this is also how an unmet layer policy surfaces). The plan-boundary statuses also return plan (the current manifest), revision (pin it into revise_plan/approve_plan), questions (open planner questions — answer them via revise_plan, echoing each question_id) templates (the ids a node's template field may take, each with its description, model and tier: cheap|mid|premium) and premium_cap (how many premium-tier nodes one plan may bind — the effective dynwf_max_premium_workers for this project). This is one snapshot: to block until the next transition instead of re-polling, use workflow_wait.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"instance_id":{"type":"string"},
"result_key":{"type":"string","description":"Session finding key to read back on completion (default workflow_final_result)"}
},
"required":["instance_id"],
"additionalProperties":false
}`),
	}
}

func (h getSubworkflowHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		InstanceID string `json:"instance_id"`
		ResultKey  string `json:"result_key"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if strings.TrimSpace(args.InstanceID) == "" {
		return "instance_id is required", true, nil
	}
	wi, err := loadGuardedInstance(h.d, env.ProjectID, args.InstanceID)
	if err != nil {
		return err.Error(), true, nil
	}
	out, err := json.Marshal(subworkflowStateFor(h.d, wi, args.ResultKey))
	if err != nil {
		return err.Error(), true, nil
	}
	return string(out), false, nil
}

// revisePlanHandler implements the console-scoped revise_plan: mirrors
// POST .../plan/revise (handlers_plan.go), project-guarded via
// loadGuardedInstance instead of the REST route's implicit any-caller access.
type revisePlanHandler struct{ d Deps }

func (revisePlanHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "revise_plan",
		Description: "Append a new revision to a plan-driven run's plan (started via dynamic_workflow), at the plan boundary. Two modes, and plan wins: set plan to store your own edited manifest (feedback and answers are then ignored), or omit plan to re-run the planner with feedback+answers. Either way the revision number increments by exactly 1, so re-read get_subworkflow before the next call. revision must match the instance's current revision or the call is rejected as stale (the error names the current one). Only a draft plan can be revised — once approved, revise_plan is rejected.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
 "instance_id":{"type":"string"},
 "revision":{"type":"integer","description":"Must match the current plan revision from get_subworkflow; pass 0 while status is planning and no draft exists yet"},
 "plan":{"type":"object","description":"Full manifest, stored as-is after validation. Exact v1 shape, no other fields accepted at any level: {\"version\":1,\"goal\":\"...\",\"layers\":[{\"layer\":0,\"policy\":\"any\",\"nodes\":[{\"id\":\"map\",\"template\":\"<an id from get_subworkflow.templates>\",\"instructions\":\"...\"}]}]}. STRUCTURE: layers must be dense and 0-indexed (a layer's layer number equals its position), run in ascending order, and nodes inside one layer run concurrently; the LAST layer must hold exactly one node — it carries the run's result. Node ids are unique across the whole plan and match ^[a-z0-9][a-z0-9_-]{0,63}$. The questions array is optional — omit it. DATA FLOW: a node sees only its own instructions, so to consume an earlier node's output put #{NODE_FINDINGS:<node-id>} (or #{NODE_FINDINGS:<node-id>:key1,key2}) inside its instructions; the target must sit in a strictly earlier layer. POLICY: the layer's pass rule — any (1 node must pass), all, quorum:N, percent:P (ceil of P% of the layer's nodes) — it gates whether the run advances past the layer and never affects which findings a later node can read. TIER: a node has NO model/effort/tools field, its template decides those, so bind a cheap-tier template to keep a worker cheap; premium-tier nodes are capped at the premium_cap get_subworkflow returns (dynwf_max_premium_workers, default 2) and over the cap this call is rejected, naming the offending nodes. FINDING KEYS: each template's description names the finding key its node emits to, which is the key to use in the #{NODE_FINDINGS:<node-id>:<key>} form. FAILURE: a layer whose policy is not met fails the whole run — status failed, with failure_reason naming the layer, its policy and the passed/required counts."},
 "feedback":{"type":"string","description":"Steer a planner re-run in prose. Ignored when plan is set."},
 "answers":{"type":"array","description":"Answers to the open questions get_subworkflow returned. Ignored when plan is set.","items":{"type":"object","properties":{"question_id":{"type":"string"},"answer":{"type":"string"}}}}
},
"required":["instance_id","revision"],
"additionalProperties":false
}`),
	}
}

func (h revisePlanHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
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
	if h.d.Orch == nil {
		return missingService("orchestrator")
	}
	wi, err := loadGuardedInstance(h.d, env.ProjectID, args.InstanceID)
	if err != nil {
		return err.Error(), true, nil
	}

	req := types.PlanReviseRequest{Revision: args.Revision, Manifest: args.Plan, Feedback: args.Feedback, Answers: args.Answers}
	rev, err := service.NewPlanService(h.d.Pool, h.d.Clock, h.d.Orch).Revise(ctx, args.InstanceID, req)
	if err != nil {
		return planErrText(h.d, args.InstanceID, err), true, nil
	}

	eventType := ws.EventPlanRevised
	if rev.Revision == 1 {
		eventType = ws.EventPlanDrafted
	}
	h.d.WSHub.Broadcast(ws.NewEvent(eventType, wi.ProjectID, wi.TicketID, wi.WorkflowID, map[string]interface{}{
		"instance_id": args.InstanceID,
		"revision":    rev.Revision,
		"author":      rev.Author,
	}))

	out, err := json.Marshal(rev)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(out), false, nil
}

// approvePlanHandler implements the console-scoped approve_plan: mirrors
// POST .../plan/approve, project-guarded via loadGuardedInstance.
type approvePlanHandler struct{ d Deps }

func (approvePlanHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "approve_plan",
		Description: "Approve a plan-driven run's plan at the given revision: materializes the manifest into agent nodes and resumes the run if it was parked at the plan boundary. Open questions never block approval — approve when the plan is good enough. revision must match the instance's current revision (see get_subworkflow) or the call is rejected as stale. The manifest is re-validated here, so approve can fail at the correct revision too: a template's model may have been disabled since the draft, or the plan may exceed dynwf_max_premium_workers — fix it with revise_plan and approve the new revision. A result of \"approved but resume failed\" means the plan IS approved and only the resume needs retrying.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
 "instance_id":{"type":"string"},
 "revision":{"type":"integer","description":"Must match the instance's current plan revision"}
},
"required":["instance_id","revision"],
"additionalProperties":false
}`),
	}
}

func (h approvePlanHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
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
	if h.d.Orch == nil {
		return missingService("orchestrator")
	}
	wi, err := loadGuardedInstance(h.d, env.ProjectID, args.InstanceID)
	if err != nil {
		return err.Error(), true, nil
	}

	rev, err := service.NewPlanService(h.d.Pool, h.d.Clock, h.d.Orch).Approve(args.InstanceID, args.Revision)
	if err != nil {
		return planErrText(h.d, args.InstanceID, err), true, nil
	}

	h.d.WSHub.Broadcast(ws.NewEvent(ws.EventPlanApproved, wi.ProjectID, wi.TicketID, wi.WorkflowID, map[string]interface{}{
		"instance_id": args.InstanceID,
		"revision":    rev.Revision,
	}))
	h.d.WSHub.Broadcast(ws.NewEvent(ws.EventPlanMaterialized, wi.ProjectID, wi.TicketID, wi.WorkflowID, map[string]interface{}{
		"instance_id": args.InstanceID,
	}))

	if refreshed, rerr := repo.NewWorkflowInstanceRepo(h.d.Pool, h.d.Clock).Get(args.InstanceID); rerr == nil && model.IsPlanSuspended(refreshed.Status) {
		if err := h.d.Orch.ResumeAfterPlanApproval(ctx, args.InstanceID); err != nil {
			return fmt.Sprintf("approved but resume failed: %s", err.Error()), true, nil
		}
	}

	out, err := json.Marshal(rev)
	if err != nil {
		return err.Error(), true, nil
	}
	return string(out), false, nil
}

// planErrText renders a plan-service error for a tool result, best-effort
// naming the instance's current revision for the stale-revision/not-draft
// sentinels so the caller can retry with the right value — mirrors
// tools_builtin.planServiceErrorText, sourced from subworkflowStateFor
// instead of env.Subworkflows.GetSubworkflow.
func planErrText(d Deps, instanceID string, err error) string {
	if !errors.Is(err, service.ErrStalePlanRevision) && !errors.Is(err, service.ErrPlanNotDraft) {
		return err.Error()
	}
	wi, werr := repo.NewWorkflowInstanceRepo(d.Pool, d.Clock).Get(instanceID)
	if werr != nil {
		return err.Error()
	}
	state := subworkflowStateFor(d, wi, "")
	if state.Revision == 0 {
		return err.Error()
	}
	return fmt.Sprintf("%s (current revision: %d)", err.Error(), state.Revision)
}
