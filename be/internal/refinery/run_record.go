package refinery

import (
	"context"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/spawner/apirun/provider"
	"be/internal/ws"
)

// foldTarget identifies the slot a fold writes to: sessionID for a console
// fold, or (workflowInstanceID, nodeID) for an autonomous fold — never both
// meaningfully populated at once.
type foldTarget struct {
	sessionID          string
	workflowInstanceID string
	nodeID             string
	foldSeq            int
}

// logKey returns the id used in log lines: the session id, or the
// workflow-instance/node slot when sessionID is empty (autonomous fold).
func (t foldTarget) logKey() string {
	if t.sessionID != "" {
		return t.sessionID
	}
	return t.workflowInstanceID + "/" + t.nodeID
}

// recordFoldRun best-effort persists a refinery_runs row for target's fold
// attempt and, on failure, broadcasts refinery.fold_failed through the
// nil-safe broadcaster seam. Never propagates an insert error — mirrors the
// package's best-effort fold invariant.
func (m *Manager) recordFoldRun(ctx context.Context, target foldTarget, projectID, provName, modelName string, usage provider.Usage, status, errMsg string) {
	run := &model.RefineryRun{
		SessionID:          target.sessionID,
		WorkflowInstanceID: target.workflowInstanceID,
		NodeID:             target.nodeID,
		ProjectID:          projectID,
		Provider:           provName,
		Model:              modelName,
		PromptTokens:       usage.InputTokens,
		OutputTokens:       usage.OutputTokens,
		Status:             status,
		Error:              errMsg,
		FoldCount:          target.foldSeq,
	}
	if err := m.runRepo.Insert(run); err != nil {
		logger.Warn(ctx, "refinery: record fold run failed", "key", target.logKey(), "error", err)
	}

	if status == "failed" && m.broadcaster != nil {
		m.broadcaster(ws.NewEvent(ws.EventRefineryFoldFailed, projectID, "", "", map[string]interface{}{
			"session_id":           target.sessionID,
			"workflow_instance_id": target.workflowInstanceID,
			"node_id":              target.nodeID,
			"provider":             provName,
			"model":                modelName,
			"error":                errMsg,
		}))
	}
}
