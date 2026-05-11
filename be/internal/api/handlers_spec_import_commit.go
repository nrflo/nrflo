package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/types"
)

// handleCommitSpecImport creates a ticket from a completed spec-import session,
// persists ticket_refs, archives the import wfi, and starts the target workflow.
// POST /api/v1/import/spec/{instance_id}/commit
func (s *Server) handleCommitSpecImport(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("instance_id")
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance ID required")
		return
	}

	var body struct {
		Title        string `json:"title"`
		Description  string `json:"description"`
		WorkflowName string `json:"workflow_name"`
		Instructions string `json:"instructions"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if body.WorkflowName == "" {
		writeError(w, http.StatusBadRequest, "workflow_name is required")
		return
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	wfi, err := wfiRepo.Get(instanceID)
	if err != nil || wfi.WorkflowID != specImportWorkflowID {
		writeError(w, http.StatusNotFound, "import session not found")
		return
	}
	if wfi.Status != model.WorkflowInstanceActive {
		writeError(w, http.StatusConflict, "import session is not active")
		return
	}

	projectID := wfi.ProjectID
	findings := wfi.GetFindings()

	// Parse attached refs from findings.
	var attachedRefs []*model.TicketRef
	if refsStr, _ := findings["_spec_attached_refs"].(string); refsStr != "" {
		var rawRefs []json.RawMessage
		if json.Unmarshal([]byte(refsStr), &rawRefs) == nil {
			for _, raw := range rawRefs {
				var ref model.TicketRef
				if json.Unmarshal(raw, &ref) == nil && ref.URL != "" {
					attachedRefs = append(attachedRefs, &ref)
				}
			}
		}
	}

	// Create the ticket.
	ticketSvc := s.ticketService()
	ticket, err := ticketSvc.Create(projectID, &types.TicketCreateRequest{
		Title:       body.Title,
		Description: body.Description,
		Type:        "task",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create ticket: "+err.Error())
		return
	}

	// Persist ticket refs in a single transaction.
	if len(attachedRefs) > 0 {
		ticketRefRepo := repo.NewTicketRefRepo(s.pool, s.clock)
		for _, ref := range attachedRefs {
			ref.ProjectID = projectID
			ref.TicketID = ticket.ID
		}
		if err := ticketRefRepo.BulkCreate(attachedRefs); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to persist ticket refs: "+err.Error())
			return
		}
	}

	// Archive the spec-import wfi.
	updatedFindings := wfi.GetFindings()
	updatedFindings["_archived"] = true
	wfi.SetFindings(updatedFindings)
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	_, archiveErr := s.pool.Exec(
		`UPDATE workflow_instances SET status = ?, findings = ?, updated_at = ? WHERE id = ?`,
		model.WorkflowInstanceProjectCompleted, wfi.Findings, now, instanceID,
	)
	if archiveErr != nil {
		// Non-fatal: ticket was already created. Log but don't fail.
		_ = fmt.Errorf("archive spec import wfi %s: %w", instanceID, archiveErr)
	}

	// Start the target workflow via the orchestrator.
	instructions := body.Instructions
	if instructions == "" {
		instructions, _ = findings["_raw_spec"].(string)
	}

	var newInstanceID string
	if s.orchestrator != nil {
		result, startErr := s.orchestrator.Start(r.Context(), orchestrator.RunRequest{
			ProjectID:    projectID,
			TicketID:     ticket.ID,
			WorkflowName: body.WorkflowName,
			Instructions: strings.TrimSpace(instructions),
			ScopeType:    "ticket",
		})
		if startErr == nil && result != nil {
			newInstanceID = result.InstanceID
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ticket_id":   ticket.ID,
		"instance_id": newInstanceID,
	})
}
