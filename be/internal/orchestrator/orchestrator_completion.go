package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// markCompleted marks the workflow instance as completed and broadcasts.
// Returns the workflow_final_result finding value (empty string if not set).
func (o *Orchestrator) markCompleted(wfiID string, req RunRequest) (finalResult string) {
	o.updateOrchestrationStatus(wfiID, "completed")

	database, err := db.Open(o.dataPath)
	if err != nil {
		return
	}
	defer database.Close()
	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)

	if req.IsProjectScope() {
		wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceProjectCompleted)
		asRepo := repo.NewAgentSessionRepo(database, o.clock)
		asRepo.UpdateStatusByWorkflowInstance(wfiID, model.AgentSessionProjectCompleted)
	} else {
		wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceCompleted)
		if req.CloseTicketOnComplete {
			ticketService := service.NewTicketService(pool, o.clock)
			ticket, err := ticketService.Get(req.ProjectID, req.TicketID)
			if err != nil {
				logger.Error(context.Background(), "failed to fetch ticket for auto-close", "ticket", req.TicketID, "err", err)
			} else if ticket.Status == model.StatusClosed {
				logger.Info(context.Background(), "skipping auto-close: ticket already closed", "ticket", req.TicketID)
			} else {
				reason := fmt.Sprintf("Workflow '%s' completed successfully", req.WorkflowName)
				if err := ticketService.Close(req.ProjectID, req.TicketID, reason); err != nil {
					logger.Error(context.Background(), "failed to close ticket", "ticket", req.TicketID, "err", err)
				} else {
					o.wsHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, req.ProjectID, req.TicketID, "", map[string]interface{}{"status": "closed"}))
					// Best-effort: auto-close parent epic if all children are now closed
					if epic, err := ticketService.TryCloseParentEpic(req.ProjectID, req.TicketID); err != nil {
						logger.Error(context.Background(), "failed to auto-close parent epic", "ticket", req.TicketID, "err", err)
					} else if epic != nil {
						o.wsHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, req.ProjectID, epic.ID, "", map[string]interface{}{"status": "closed"}))
					}
				}
			}
		}
	}

	finalResult = service.ExtractWorkflowFinalResultByInstanceID(pool, wfiID, o.clock)
	data := map[string]interface{}{"instance_id": wfiID}
	if finalResult != "" {
		data["workflow_final_result"] = finalResult
	}
	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationCompleted, req.ProjectID, req.TicketID, req.WorkflowName, data))
	if req.WorkflowName == ws.SpecImportWorkflowID {
		o.wsHub.Broadcast(ws.NewEvent(ws.EventSpecImportReady, req.ProjectID, "", req.WorkflowName, map[string]interface{}{
			"instance_id": wfiID,
		}))
	}
	return
}

// markFailed marks the workflow instance as failed and broadcasts.
func (o *Orchestrator) markFailed(wfiID string, req RunRequest, reason string) {
	o.updateOrchestrationStatus(wfiID, "failed")

	database, err := db.Open(o.dataPath)
	if err != nil {
		return
	}
	defer database.Close()
	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, o.clock)
	wfiRepo.UpdateStatus(wfiID, model.WorkflowInstanceFailed)

	// Revert ticket from in_progress to open so it's not stuck (ticket scope only)
	if !req.IsProjectScope() {
		ticketService := service.NewTicketService(pool, o.clock)
		if err := ticketService.Reopen(req.ProjectID, req.TicketID); err != nil {
			logger.Error(context.Background(), "failed to reopen ticket after failure", "ticket", req.TicketID, "err", err)
		} else {
			o.wsHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, req.ProjectID, req.TicketID, "", map[string]interface{}{"status": "open"}))
		}
	}

	if o.errorSvc != nil {
		o.errorSvc.RecordError(req.ProjectID, "workflow", wfiID, reason)
	}

	o.wsHub.Broadcast(ws.NewEvent(ws.EventOrchestrationFailed, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"instance_id": wfiID,
		"reason":      reason,
	}))
	if req.WorkflowName == ws.SpecImportWorkflowID {
		o.wsHub.Broadcast(ws.NewEvent(ws.EventSpecImportFailed, req.ProjectID, "", req.WorkflowName, map[string]interface{}{
			"instance_id": wfiID,
			"error":       reason,
		}))
	}

	// Persist _failure_reason finding for agent visibility.
	findingRepo := repo.NewFindingRepo(pool, o.clock)
	if val, err := json.Marshal(map[string]interface{}{"reason": reason}); err == nil {
		_ = findingRepo.Upsert("workflow_instance", wfiID, "_failure_reason", val,
			repo.Denorm{WorkflowInstanceID: wfiID},
			repo.Actor{Source: "orchestrator"})
	}

	if reason != reasonCancelled {
		o.runFinalize(context.Background(), wfiID, req, outcomeFailure, reason)
	}

	// Purge sensitive trace data when the workflow opted in (after the failure-finalize slot).
	o.maybePurgeTrace(wfiID)
}

// failReasonOr returns the custom failReason set on the runState (by FailWorkflow before cancel),
// falling back to fallback when no custom reason was set.
func (o *Orchestrator) failReasonOr(wfiID, fallback string) string {
	o.mu.Lock()
	rs := o.runs[wfiID]
	o.mu.Unlock()
	if rs != nil && rs.failReason != "" {
		return rs.failReason
	}
	return fallback
}

// updateOrchestrationStatus updates the _orchestration key in findings.
func (o *Orchestrator) updateOrchestrationStatus(wfiID, status string) {
	database, err := db.Open(o.dataPath)
	if err != nil {
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	findingRepo := repo.NewFindingRepo(pool, o.clock)
	val, _ := json.Marshal(map[string]interface{}{"status": status})
	findingRepo.Upsert("workflow_instance", wfiID, "_orchestration", val, //nolint:errcheck
		repo.Denorm{WorkflowInstanceID: wfiID},
		repo.Actor{Source: "orchestrator"})
}
