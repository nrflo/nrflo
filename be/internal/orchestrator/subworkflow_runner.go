package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun"
)

// StartSubworkflow starts a callable workflow as a detached, project-scoped child
// run under the caller's project and returns its instance id immediately (async;
// poll with GetSubworkflow). It implements the start half of apirun.SubworkflowRunner
// for the run_subworkflow builtin.
//
// Guards (shared with StartDynamicWorkflow via startChildRun): the def must be
// callable_as_subworkflow (and, by validation, non-purging with no pause
// layers), the child's subworkflow_depth must stay under subworkflow_max_depth,
// the parent instance carries a persisted invocation budget (survives
// pause/continue and retry), and a global concurrent-children cap is reserved
// under o.mu. A watcher goroutine ties the child to the parent run: children
// are stopped when the parent reaches a terminal status (a paused parent
// re-arms the watcher instead — see subworkflow_watch.go).
func (o *Orchestrator) StartSubworkflow(ctx context.Context, parentInstanceID, projectID, workflowName, instructions string) (string, error) {
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return "", fmt.Errorf("run_subworkflow: open pool: %w", err)
	}
	defer pool.Close()

	return o.startChildRun(ctx, pool, parentInstanceID, projectID, workflowName, instructions, false)
}

// chargeSubworkflowStart atomically consumes one unit of the parent's persisted
// invocation budget; it fails when the budget is exhausted.
func (o *Orchestrator) chargeSubworkflowStart(pool *db.Pool, parentInstanceID string, maxInvocations int) error {
	res, err := pool.Exec(
		`UPDATE workflow_instances SET subworkflow_starts = subworkflow_starts + 1 WHERE id = ? AND subworkflow_starts < ?`,
		parentInstanceID, maxInvocations)
	if err != nil {
		return fmt.Errorf("run_subworkflow: charge budget: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("run_subworkflow: invocation budget exhausted (%d per run, subworkflow_max_invocations)", maxInvocations)
	}
	return nil
}

// refundSubworkflowStart returns a budget unit after a failed start (best-effort).
func (o *Orchestrator) refundSubworkflowStart(pool *db.Pool, parentInstanceID string) {
	_, _ = pool.Exec(
		`UPDATE workflow_instances SET subworkflow_starts = subworkflow_starts - 1 WHERE id = ? AND subworkflow_starts > 0`,
		parentInstanceID)
}

// GetSubworkflow returns a child run's status ("running", "waiting", the four
// plan-boundary statuses, "completed", or "failed") and, depending on status,
// its payload: the session finding named resultKey (default
// "workflow_final_result") for completed runs, the failure reason for failed
// ones, or the current plan draft (Plan/Revision/Questions) for the four
// plan-boundary statuses. It implements the poll half of
// apirun.SubworkflowRunner. Only the run that started a child (its persisted
// parent_instance_id) may read it back.
func (o *Orchestrator) GetSubworkflow(ctx context.Context, callerInstanceID, projectID, instanceID, resultKey string) (apirun.SubworkflowState, error) {
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return apirun.SubworkflowState{}, fmt.Errorf("get_subworkflow: open pool: %w", err)
	}
	defer pool.Close()

	wi, err := o.assertChildOwnership(pool, callerInstanceID, projectID, instanceID)
	if err != nil {
		return apirun.SubworkflowState{}, err
	}

	findingRepo := repo.NewFindingRepo(pool, o.clock)
	switch wi.Status {
	case model.WorkflowInstanceCompleted, model.WorkflowInstanceProjectCompleted:
		key := resultKey
		if key == "" {
			key = "workflow_final_result"
		}
		val, _ := findingRepo.GetSessionFindingByKey(instanceID, key)
		return apirun.SubworkflowState{Status: "completed", Result: val}, nil
	case model.WorkflowInstanceFailed:
		reason := ""
		if own, ferr := findingRepo.GetOwn("workflow_instance", instanceID); ferr == nil {
			if raw, ok := own["_failure_reason"]; ok {
				var fr struct {
					Reason string `json:"reason"`
				}
				if json.Unmarshal(raw, &fr) == nil {
					reason = fr.Reason
				}
			}
		}
		return apirun.SubworkflowState{Status: "failed", FailureReason: reason}, nil
	case model.WorkflowInstanceWaiting:
		// Reachable despite the no-pause start guard (e.g. the def gained a pause
		// layer mid-run): surface it so pollers terminate instead of spinning.
		return apirun.SubworkflowState{Status: "waiting", FailureReason: "paused after a pause_after layer; requires human resume and will not complete for this caller"}, nil
	case model.WorkflowInstancePlanning, model.WorkflowInstancePlanReady, model.WorkflowInstanceWaitingInput, model.WorkflowInstanceWaitingApproval:
		// Plan-driven runs are callable (unlike pause_after): the caller must
		// drive the plan lifecycle (revise/approve) rather than poll forever.
		state := apirun.SubworkflowState{Status: string(wi.Status)}
		if draft, derr := service.NewPlanService(pool, o.clock, o).GetDraft(instanceID); derr == nil {
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
		state.Templates = service.PlanTemplateChoicesJSON(pool, o.clock, wi.ProjectID, wi.WorkflowID)
		return state, nil
	default:
		return apirun.SubworkflowState{Status: "running"}, nil
	}
}
