package orchestrator

import (
	"context"
	"fmt"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/service"
	"be/internal/spawner"
)

// drainCallbackPlanStep checks for an active callback-plan step and, if
// present, executes it: spawns its phases, tallies pass/fail, re-enters
// handleCallback on a nested callback, applies the layer's pass policy, and
// advances the plan index (resuming forward iteration once the plan
// completes). Extracted from runLoop to keep orchestrator_loop.go under its
// filesize.baseline ceiling.
//
// hasStep is false when there is no active plan step — the caller should
// proceed to forward iteration. When hasStep is true, shouldReturn tells
// runLoop to return immediately (the workflow was already marked
// failed/paused by this call); worktreeHandled mirrors runLoop's own flag in
// that case. Otherwise the caller continues its loop using newLayerIdx.
func (o *Orchestrator) drainCallbackPlanStep(
	ctx context.Context,
	wfiID string,
	req RunRequest,
	layerGroups []layerGroup,
	layerIdx int,
	parentSession string,
	baseCfg spawner.Config,
	layerPolicies map[int]string,
	layerPause map[int]bool,
	pool *db.Pool,
	projectRoot string,
	callbackCount *int,
) (hasStep bool, newLayerIdx int, shouldReturn bool, worktreeHandled bool) {
	o.mu.Lock()
	var planStep callbackPlanStep
	hasPlanStep := false
	if rs := o.runs[wfiID]; rs != nil && rs.callbackPlanIdx < len(rs.callbackPlan.steps) {
		planStep = rs.callbackPlan.steps[rs.callbackPlanIdx]
		hasPlanStep = true
	}
	o.mu.Unlock()

	if !hasPlanStep {
		return false, layerIdx, false, false
	}

	stepIdx := layerIndexOf(planStep.layer, layerGroups)
	var stepPhases []service.SpawnerPhaseDef
	if stepIdx >= 0 {
		if planStep.wholeLayer {
			stepPhases = layerGroups[stepIdx].phases
		} else {
			nodeSet := make(map[string]bool, len(planStep.nodes))
			for _, n := range planStep.nodes {
				nodeSet[n] = true
			}
			for _, p := range layerGroups[stepIdx].phases {
				if nodeSet[p.NodeID] {
					stepPhases = append(stepPhases, p)
				}
			}
		}
	}

	logger.Info(ctx, "executing plan step", "layer", planStep.layer, "whole_layer", planStep.wholeLayer, "agents", len(stepPhases))
	stepResults := o.spawnPhases(ctx, wfiID, req, stepPhases, parentSession, baseCfg)

	passCount, failCount := 0, 0
	var stepCBErrs []*spawner.CallbackError
	for _, r := range stepResults {
		switch {
		case r.callbackErr != nil:
			if !planStep.wholeLayer {
				o.markFailed(wfiID, req, "callback within agent/chain plan step is not supported in v1")
				return true, layerIdx, true, false
			}
			passCount++
			stepCBErrs = append(stepCBErrs, r.callbackErr)
		case r.err != nil:
			if ctx.Err() != nil {
				o.markFailed(wfiID, req, o.failReasonOr(wfiID, reasonCancelled))
				return true, layerIdx, true, false
			}
			logger.Error(ctx, "plan step agent failed", "layer", planStep.layer, "agent", r.agent, "err", r.err)
			failCount++
		default:
			logger.Info(ctx, "plan step agent completed", "layer", planStep.layer, "agent", r.agent)
			passCount++
		}
	}

	if len(stepCBErrs) > 0 {
		// Whole-layer plan step triggered another callback: re-enter handleCallback
		if !o.handleCallback(ctx, wfiID, req, layerGroups, stepIdx, stepCBErrs, callbackCount) {
			return true, layerIdx, true, false
		}
		return true, layerIdx, false, false
	}

	denom := passCount + failCount
	if denom > 0 {
		policy, _ := service.ParseLayerPolicy(layerPolicies[planStep.layer])
		required := policy.Required(denom)
		if passCount < required {
			logger.Error(ctx, "plan step pass_policy not satisfied", "layer", planStep.layer,
				"policy", policy.String(), "passed", passCount, "total", denom, "required", required)
			o.markFailed(wfiID, req, fmt.Sprintf(
				"plan step layer %d: pass_policy %q not satisfied (%d/%d passed, %d required)",
				planStep.layer, policy.String(), passCount, denom, required))
			return true, layerIdx, true, false
		}
	}

	// Advance plan index; finalize plan when all steps are done
	o.mu.Lock()
	rs := o.runs[wfiID]
	if rs == nil {
		o.mu.Unlock()
		return true, layerIdx, false, false
	}
	rs.callbackPlanIdx++
	if rs.callbackPlanIdx < len(rs.callbackPlan.steps) {
		o.mu.Unlock()
		return true, layerIdx, false, false
	}
	resumeLayer := rs.callbackPlan.resumeLayer
	rs.callbackPlan = callbackPlan{}
	rs.callbackPlanIdx = 0
	o.mu.Unlock()

	o.clearCallbackMetadata(ctx, wfiID)
	newIdx := layerIndexOf(resumeLayer, layerGroups)
	if newIdx < 0 {
		newIdx = len(layerGroups) // resumeLayer past end → exit loop
	}
	if newIdx > 0 && o.maybePauseAfterLayer(ctx, wfiID, req, newIdx-1, layerGroups, layerPause, pool, projectRoot) {
		return true, newIdx, true, true
	}
	return true, newIdx, false, false
}
