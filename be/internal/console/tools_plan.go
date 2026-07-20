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
		Description: "Poll a workflow instance's status: running/waiting/completed/failed, or (for a plan-driven run) the current plan draft (plan/revision/questions) to drive via revise_plan/approve_plan.",
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
		Description: "Revise a workflow instance's plan (started via dynamic_workflow) at the plan boundary: supply an edited manifest (plan) or feedback/answers to re-run the planner. revision must match the instance's current revision (see get_subworkflow) or the call is rejected as stale.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
 "instance_id":{"type":"string"},
 "revision":{"type":"integer","description":"Must match the instance's current plan revision (0 if it has none yet)"},
 "plan":{"type":"object","description":"A full, edited plan manifest (version/goal/layers/questions) to store as-is after validation"},
 "feedback":{"type":"string","description":"Feedback for the planner to re-run with (used when plan is omitted)"},
 "answers":{"type":"array","description":"Answers to open plan questions","items":{"type":"object","properties":{"question_id":{"type":"string"},"answer":{"type":"string"}}}}
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
		Description: "Approve a workflow instance's plan at the given revision (materializes it and resumes the run if it was parked at the plan boundary). revision must match the instance's current revision (see get_subworkflow) or the call is rejected as stale.",
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
