package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// StartSubworkflow starts a callable workflow as a detached, project-scoped child
// run under the caller's project and returns its instance id immediately (async;
// poll with GetSubworkflow). It implements the start half of apirun.SubworkflowRunner
// for the run_subworkflow builtin.
//
// Guards: the def must be callable_as_subworkflow (and, by validation, non-purging
// with no pause layers), the child's subworkflow_depth must stay under
// subworkflow_max_depth, the parent instance carries a persisted invocation budget
// (survives pause/continue and retry), and a global concurrent-children cap is
// reserved under o.mu. A watcher goroutine ties the child to the parent run:
// children are stopped when the parent reaches a terminal status (a paused parent
// re-arms the watcher instead — see subworkflow_watch.go).
func (o *Orchestrator) StartSubworkflow(ctx context.Context, parentInstanceID, projectID, workflowName, instructions string) (string, error) {
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return "", fmt.Errorf("run_subworkflow: open pool: %w", err)
	}
	defer pool.Close()

	parent, err := repo.NewWorkflowInstanceRepo(pool, o.clock).Get(parentInstanceID)
	if err != nil {
		return "", fmt.Errorf("run_subworkflow: load parent instance: %w", err)
	}

	dbWorkflow, _, defProjectID, err := o.resolveWorkflowDef(pool, projectID, workflowName)
	if err != nil {
		return "", fmt.Errorf("run_subworkflow: %w", err)
	}
	if !dbWorkflow.CallableAsSubworkflow {
		return "", fmt.Errorf("run_subworkflow: workflow %q is not callable_as_subworkflow", workflowName)
	}
	if dbWorkflow.PurgeOnCompletion {
		return "", fmt.Errorf("run_subworkflow: workflow %q has purge_on_completion; its result would be purged before readback", workflowName)
	}
	pause, err := service.NewWorkflowLayerPolicyService(pool, o.clock).GetLayerPauseAfter(defProjectID, dbWorkflow.ID)
	if err != nil {
		return "", fmt.Errorf("run_subworkflow: load pause layers: %w", err)
	}
	for layer, p := range pause {
		if p {
			return "", fmt.Errorf("run_subworkflow: workflow %q pauses after layer %d; paused runs never terminate for the caller", workflowName, layer)
		}
	}

	depth := parent.SubworkflowDepth + 1
	if maxDepth := service.SubworkflowCap(pool, projectID, service.SubworkflowMaxDepthKey, service.DefaultSubworkflowMaxDepth); depth > maxDepth {
		return "", fmt.Errorf("run_subworkflow: nesting depth %d exceeds subworkflow_max_depth %d", depth, maxDepth)
	}
	maxChildren := service.SubworkflowCap(pool, projectID, service.SubworkflowMaxChildrenKey, service.DefaultSubworkflowMaxChildren)
	maxInvocations := service.SubworkflowCap(pool, projectID, service.SubworkflowMaxInvocationsKey, service.DefaultSubworkflowMaxInvocations)

	// Persisted invocation budget: an atomic conditional increment on the parent
	// row, so it survives pause/continue and retry-failed (the in-memory runState
	// is recreated on those paths).
	if err := o.chargeSubworkflowStart(pool, parentInstanceID, maxInvocations); err != nil {
		return "", err
	}

	// Reserve a concurrency slot; the parent must still be live for the cascade.
	o.mu.Lock()
	rs := o.runs[parentInstanceID]
	if rs == nil {
		o.mu.Unlock()
		o.refundSubworkflowStart(pool, parentInstanceID)
		return "", fmt.Errorf("run_subworkflow: parent run %s is not active", parentInstanceID)
	}
	if o.subworkflowActive >= maxChildren {
		o.mu.Unlock()
		o.refundSubworkflowStart(pool, parentInstanceID)
		return "", fmt.Errorf("run_subworkflow: %d sub-workflows already running (subworkflow_max_children)", maxChildren)
	}
	o.subworkflowActive++
	parentDone := rs.done
	o.mu.Unlock()

	// Detached start: the caller's tool ctx must not cancel the child (async contract).
	res, err := o.Start(context.Background(), RunRequest{
		ProjectID:        projectID,
		WorkflowName:     workflowName,
		ScopeType:        "project",
		Instructions:     instructions,
		LaunchDepth:      parent.LaunchDepth + 1,
		ParentInstanceID: parentInstanceID,
		SubworkflowDepth: depth,
	})
	if err != nil {
		o.mu.Lock()
		o.subworkflowActive--
		o.mu.Unlock()
		o.refundSubworkflowStart(pool, parentInstanceID)
		return "", fmt.Errorf("run_subworkflow: start failed: %w", err)
	}

	go o.watchSubworkflow(parentInstanceID, parentDone, res.InstanceID, true)
	logger.Info(ctx, "sub-workflow started", "workflow", workflowName, "instance_id", res.InstanceID, "parent", parentInstanceID, "depth", depth)
	return res.InstanceID, nil
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

// GetSubworkflow returns a child run's status ("running", "waiting", "completed"
// or "failed") and, when terminal, its result: the session finding named resultKey
// (default "workflow_final_result") for completed runs, or the failure reason for
// failed ones. It implements the poll half of apirun.SubworkflowRunner. Only the
// run that started a child (its persisted parent_instance_id) may read it back.
func (o *Orchestrator) GetSubworkflow(ctx context.Context, callerInstanceID, projectID, instanceID, resultKey string) (status string, result json.RawMessage, failureReason string, err error) {
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return "", nil, "", fmt.Errorf("get_subworkflow: open pool: %w", err)
	}
	defer pool.Close()

	wi, err := repo.NewWorkflowInstanceRepo(pool, o.clock).Get(instanceID)
	if err != nil {
		return "", nil, "", fmt.Errorf("get_subworkflow: %w", err)
	}
	if !strings.EqualFold(wi.ProjectID, projectID) ||
		wi.ParentInstanceID == "" || !strings.EqualFold(wi.ParentInstanceID, callerInstanceID) {
		return "", nil, "", fmt.Errorf("get_subworkflow: %s was not started by this run", instanceID)
	}

	findingRepo := repo.NewFindingRepo(pool, o.clock)
	switch wi.Status {
	case model.WorkflowInstanceCompleted, model.WorkflowInstanceProjectCompleted:
		key := resultKey
		if key == "" {
			key = "workflow_final_result"
		}
		val, _ := findingRepo.GetSessionFindingByKey(instanceID, key)
		return "completed", val, "", nil
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
		return "failed", nil, reason, nil
	case model.WorkflowInstanceWaiting:
		// Reachable despite the no-pause start guard (e.g. the def gained a pause
		// layer mid-run): surface it so pollers terminate instead of spinning.
		return "waiting", nil, "paused after a pause_after layer; requires human resume and will not complete for this caller", nil
	default:
		return "running", nil, "", nil
	}
}
