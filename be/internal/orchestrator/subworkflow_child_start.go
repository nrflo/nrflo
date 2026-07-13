package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// startChildRun is the guard/charge/slot/watcher block shared by
// StartSubworkflow and StartDynamicWorkflow: callable_as_subworkflow/purge/
// pause-layer checks, the subworkflow_depth cap, the persisted invocation
// budget, a concurrent-children slot reserved under o.mu, a detached o.Start,
// and the parent-death watcher — so both starters enforce the same caps
// identically. planAuto sets RunRequest.PlanAutoApprove so a plan-driven child
// materializes without suspending once its plan is drafted (StartDynamicWorkflow
// mode=auto only; StartSubworkflow always passes false).
func (o *Orchestrator) startChildRun(ctx context.Context, pool *db.Pool, parentInstanceID, projectID, workflowName, instructions string, planAuto bool) (string, error) {
	parent, err := repo.NewWorkflowInstanceRepo(pool, o.clock).Get(parentInstanceID)
	if err != nil {
		return "", fmt.Errorf("sub-workflow start: load parent instance: %w", err)
	}

	dbWorkflow, _, defProjectID, err := o.resolveWorkflowDef(pool, projectID, workflowName)
	if err != nil {
		return "", fmt.Errorf("sub-workflow start: %w", err)
	}
	if !dbWorkflow.CallableAsSubworkflow {
		return "", fmt.Errorf("sub-workflow start: workflow %q is not callable_as_subworkflow", workflowName)
	}
	if dbWorkflow.PurgeOnCompletion {
		return "", fmt.Errorf("sub-workflow start: workflow %q has purge_on_completion; its result would be purged before readback", workflowName)
	}
	pause, err := service.NewWorkflowLayerPolicyService(pool, o.clock).GetLayerPauseAfter(defProjectID, dbWorkflow.ID)
	if err != nil {
		return "", fmt.Errorf("sub-workflow start: load pause layers: %w", err)
	}
	for layer, p := range pause {
		if p {
			return "", fmt.Errorf("sub-workflow start: workflow %q pauses after layer %d; paused runs never terminate for the caller", workflowName, layer)
		}
	}

	depth := parent.SubworkflowDepth + 1
	if maxDepth := service.SubworkflowCap(pool, projectID, service.SubworkflowMaxDepthKey, service.DefaultSubworkflowMaxDepth); depth > maxDepth {
		return "", fmt.Errorf("sub-workflow start: nesting depth %d exceeds subworkflow_max_depth %d", depth, maxDepth)
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
		return "", fmt.Errorf("sub-workflow start: parent run %s is not active", parentInstanceID)
	}
	if o.subworkflowActive >= maxChildren {
		o.mu.Unlock()
		o.refundSubworkflowStart(pool, parentInstanceID)
		return "", fmt.Errorf("sub-workflow start: %d sub-workflows already running (subworkflow_max_children)", maxChildren)
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
		PlanAutoApprove:  planAuto,
	})
	if err != nil {
		o.mu.Lock()
		o.subworkflowActive--
		o.mu.Unlock()
		o.refundSubworkflowStart(pool, parentInstanceID)
		return "", fmt.Errorf("sub-workflow start: start failed: %w", err)
	}

	go o.watchSubworkflow(parentInstanceID, parentDone, res.InstanceID, true)
	logger.Info(ctx, "sub-workflow started", "workflow", workflowName, "instance_id", res.InstanceID, "parent", parentInstanceID, "depth", depth)
	return res.InstanceID, nil
}

// assertChildOwnership loads instanceID and verifies it lives under projectID
// and was started by callerInstanceID (its persisted parent_instance_id) — the
// authorization check shared by GetSubworkflow, RevisePlan, and ApprovePlan:
// only the run that started a child may read it back or drive its plan
// lifecycle.
func (o *Orchestrator) assertChildOwnership(pool *db.Pool, callerInstanceID, projectID, instanceID string) (*model.WorkflowInstance, error) {
	wi, err := repo.NewWorkflowInstanceRepo(pool, o.clock).Get(instanceID)
	if err != nil {
		return nil, fmt.Errorf("sub-workflow: %w", err)
	}
	if !strings.EqualFold(wi.ProjectID, projectID) ||
		wi.ParentInstanceID == "" || !strings.EqualFold(wi.ParentInstanceID, callerInstanceID) {
		return nil, fmt.Errorf("sub-workflow: %s was not started by this run", instanceID)
	}
	return wi, nil
}
