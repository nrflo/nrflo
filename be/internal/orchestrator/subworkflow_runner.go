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
// with no pause layers), the child depth must stay under subworkflow_max_depth,
// the parent run has an invocation budget, and a global concurrent-children cap is
// reserved atomically with the o.runs bookkeeping. A watcher goroutine ties the
// child to the parent run: when the parent run ends first, the child is stopped
// (descendants do not outlive their root).
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

	depth := parent.LaunchDepth + 1
	if maxDepth := service.SubworkflowCap(pool, projectID, service.SubworkflowMaxDepthKey, service.DefaultSubworkflowMaxDepth); depth > maxDepth {
		return "", fmt.Errorf("run_subworkflow: nesting depth %d exceeds subworkflow_max_depth %d", depth, maxDepth)
	}
	maxChildren := service.SubworkflowCap(pool, projectID, service.SubworkflowMaxChildrenKey, service.DefaultSubworkflowMaxChildren)
	maxInvocations := service.SubworkflowCap(pool, projectID, service.SubworkflowMaxInvocationsKey, service.DefaultSubworkflowMaxInvocations)

	// Reserve budget + concurrency slots atomically; roll back on any later error.
	var parentDone chan struct{}
	o.mu.Lock()
	rs := o.runs[parentInstanceID]
	if rs == nil {
		o.mu.Unlock()
		return "", fmt.Errorf("run_subworkflow: parent run %s is not active", parentInstanceID)
	}
	if rs.subworkflowStarts >= maxInvocations {
		o.mu.Unlock()
		return "", fmt.Errorf("run_subworkflow: invocation budget exhausted (%d per run, subworkflow_max_invocations)", maxInvocations)
	}
	if o.subworkflowActive >= maxChildren {
		o.mu.Unlock()
		return "", fmt.Errorf("run_subworkflow: %d sub-workflows already running (subworkflow_max_children)", maxChildren)
	}
	rs.subworkflowStarts++
	o.subworkflowActive++
	parentDone = rs.done
	o.mu.Unlock()

	// Detached start: the caller's tool ctx must not cancel the child (async contract).
	res, err := o.Start(context.Background(), RunRequest{
		ProjectID:    projectID,
		WorkflowName: workflowName,
		ScopeType:    "project",
		Instructions: instructions,
		LaunchDepth:  depth,
	})
	if err != nil {
		o.mu.Lock()
		rs.subworkflowStarts--
		o.subworkflowActive--
		o.mu.Unlock()
		return "", fmt.Errorf("run_subworkflow: start failed: %w", err)
	}

	go o.watchSubworkflow(parentDone, res.InstanceID)
	logger.Info(ctx, "sub-workflow started", "workflow", workflowName, "instance_id", res.InstanceID, "parent", parentInstanceID, "depth", depth)
	return res.InstanceID, nil
}

// watchSubworkflow releases the child's concurrency slot when it ends, and stops
// the child if the parent run ends first (cascade — descendants do not outlive
// their root; note a paused parent also releases its run slot and thus cascades).
func (o *Orchestrator) watchSubworkflow(parentDone chan struct{}, childID string) {
	o.mu.Lock()
	var childDone chan struct{}
	if rs := o.runs[childID]; rs != nil {
		childDone = rs.done
	}
	o.mu.Unlock()

	if childDone != nil {
		select {
		case <-childDone:
		case <-parentDone:
			logger.Info(context.Background(), "parent run ended; stopping sub-workflow", "instance_id", childID)
			_ = o.Stop(childID)
			<-childDone
		}
	}
	o.mu.Lock()
	o.subworkflowActive--
	o.mu.Unlock()
}

// GetSubworkflow returns a child run's status ("running", "completed" or "failed")
// and, when terminal, its result: the session finding named resultKey (default
// "workflow_final_result") for completed runs, or the failure reason for failed
// ones. It implements the poll half of apirun.SubworkflowRunner. Only sub-runs
// (launch_depth > 0) of the caller's project are readable.
func (o *Orchestrator) GetSubworkflow(ctx context.Context, projectID, instanceID, resultKey string) (status string, result json.RawMessage, failureReason string, err error) {
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return "", nil, "", fmt.Errorf("get_subworkflow: open pool: %w", err)
	}
	defer pool.Close()

	wi, err := repo.NewWorkflowInstanceRepo(pool, o.clock).Get(instanceID)
	if err != nil {
		return "", nil, "", fmt.Errorf("get_subworkflow: %w", err)
	}
	if !strings.EqualFold(wi.ProjectID, projectID) || wi.LaunchDepth == 0 {
		return "", nil, "", fmt.Errorf("get_subworkflow: %s is not a sub-workflow of project %s", instanceID, projectID)
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
	default: // active (waiting is unreachable: callable defs reject pause layers)
		return "running", nil, "", nil
	}
}
