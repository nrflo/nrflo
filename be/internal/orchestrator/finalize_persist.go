package orchestrator

import (
	"encoding/json"
	"fmt"
	"time"

	"be/internal/db"
	"be/internal/repo"
	"be/internal/ws"
)

func persistFinalizeFinding(o *Orchestrator, pool *db.Pool, wfiID string, req RunRequest, slot, kind, target string, exitCode int, status, outputTail string) {
	val, _ := json.Marshal(map[string]interface{}{
		"slot":        slot,
		"kind":        kind,
		"target":      target,
		"exit_code":   exitCode,
		"status":      status,
		"output_tail": outputTail,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	})
	findingRepo := repo.NewFindingRepo(pool, o.clock)
	_ = findingRepo.Upsert("workflow_instance", wfiID, "_finalize", val,
		repo.Denorm{WorkflowInstanceID: wfiID},
		repo.Actor{Source: "orchestrator"})

	payload := map[string]interface{}{
		"instance_id": wfiID,
		"slot":        slot,
		"kind":        kind,
		"exit_code":   exitCode,
		"output_tail": outputTail,
	}
	if status == "ok" {
		o.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowFinalizeSucceeded, req.ProjectID, req.TicketID, req.WorkflowName, payload))
	} else {
		if o.errorSvc != nil {
			o.errorSvc.RecordError(req.ProjectID, "workflow", wfiID, fmt.Sprintf("finalize %s %s: %s", slot, status, outputTail))
		}
		o.wsHub.Broadcast(ws.NewEvent(ws.EventWorkflowFinalizeFailed, req.ProjectID, req.TicketID, req.WorkflowName, payload))
	}
}
