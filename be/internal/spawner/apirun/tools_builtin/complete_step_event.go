package tools_builtin

import (
	"be/internal/service"
	"be/internal/service/stepengine"
	"be/internal/spawner/apirun"
	"be/internal/ws"
)

// broadcastStepAdvanced is the single emission point for ws.EventStepAdvanced,
// called from each complete_step outcome leg (advance/done/rotate/counting-
// rejection) via the standard BroadcastFromCtx seam.
func broadcastStepAdvanced(env apirun.ToolEnv, stepID string, stepIndex, total, rejectedCount int, rotated bool) {
	data := map[string]interface{}{
		"workflow_instance_id": env.WorkflowInstanceID,
		"node_id":              env.NodeID,
		"step_id":              stepID,
		"step_index":           stepIndex,
		"total":                total,
		"rejected_count":       rejectedCount,
		"rotated":              rotated,
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventStepAdvanced, service.BroadcastCtx{
		ProjectID: env.ProjectID,
		TicketID:  env.TicketID,
		Workflow:  env.WorkflowName,
		SessionID: env.SessionID,
	}, data)
}

// stepTotal resolves len(steps) for the live cursor, falling back to
// outcome.CurrentIndex+1 (renderNext's original inline fallback) when the
// cursor can't be re-read.
func stepTotal(engine *stepengine.Engine, env apirun.ToolEnv, fallback int) int {
	if state, err := engine.State(env.WorkflowInstanceID, env.NodeID); err == nil {
		return len(state.Steps)
	}
	return fallback
}
