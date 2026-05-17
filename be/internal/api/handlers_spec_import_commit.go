package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// handleCommitSpecImport creates a ticket from a completed spec-import session,
// persists ticket_refs, and archives the import wfi. Workflow selection and
// execution happen separately from the ticket page after commit.
// POST /api/v1/import/spec/{instance_id}/commit
func (s *Server) handleCommitSpecImport(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("instance_id")
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance ID required")
		return
	}

	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	wfi, err := wfiRepo.Get(instanceID)
	if err != nil || wfi.WorkflowID != specImportWorkflowID {
		writeError(w, http.StatusNotFound, "import session not found")
		return
	}
	projectID := wfi.ProjectID
	findingRepo := repo.NewFindingRepo(s.pool, s.clock)
	rawFindings, _ := findingRepo.GetOwn("workflow_instance", instanceID)
	findings := rawFindingsToInterface(rawFindings)
	if archived, _ := findings["_archived"].(bool); archived {
		writeError(w, http.StatusConflict, "import session already committed")
		return
	}
	// active = fallback path (no orchestrator); project_completed = normalizer
	// agent finished. Anything else means the session is failed or still running.
	if wfi.Status != model.WorkflowInstanceActive && wfi.Status != model.WorkflowInstanceProjectCompleted {
		writeError(w, http.StatusConflict, "import session is not ready: "+string(wfi.Status))
		return
	}

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
	archivedVal, _ := json.Marshal(true)
	_ = findingRepo.Upsert("workflow_instance", instanceID, "_archived", archivedVal,
		repo.Denorm{ProjectID: projectID, WorkflowInstanceID: instanceID},
		repo.Actor{Source: "system"})
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	_, archiveErr := s.pool.Exec(
		`UPDATE workflow_instances SET status = ?, updated_at = ? WHERE id = ?`,
		model.WorkflowInstanceProjectCompleted, now, instanceID,
	)
	if archiveErr != nil {
		// Non-fatal: ticket was already created. Log but don't fail.
		_ = fmt.Errorf("archive spec import wfi %s: %w", instanceID, archiveErr)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ticket_id": ticket.ID,
	})
}
