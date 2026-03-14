package api

import (
	"context"
	"net/http"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// UpdateTicketRequest represents the request body for updating a ticket
type UpdateTicketRequest struct {
	Title          *string `json:"title,omitempty"`
	Description    *string `json:"description,omitempty"`
	Status         *string `json:"status,omitempty"`
	Priority       *int    `json:"priority,omitempty"`
	IssueType      *string `json:"issue_type,omitempty"`
	ParentTicketID *string `json:"parent_ticket_id,omitempty"`
}

// handleUpdateTicket updates a ticket
func (s *Server) handleUpdateTicket(w http.ResponseWriter, r *http.Request) {
	ticketRepo := s.ticketRepo()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)

	var req UpdateTicketRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fields := &repo.UpdateFields{
		Title:          req.Title,
		Description:    req.Description,
		Status:         req.Status,
		Priority:       req.Priority,
		IssueType:      req.IssueType,
		ParentTicketID: req.ParentTicketID,
	}

	if err := ticketRepo.Update(projectID, id, fields); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Fetch updated ticket
	updated, err := ticketRepo.Get(projectID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, updated)

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventTicketUpdated, projectID, id, "", map[string]interface{}{
			"status": string(updated.Status),
		})
		s.wsHub.Broadcast(event)
	}
}

// handleDeleteTicket deletes a ticket
func (s *Server) handleDeleteTicket(w http.ResponseWriter, r *http.Request) {
	ticketRepo := s.ticketRepo()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)
	if err := ticketRepo.Delete(projectID, id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "ticket deleted"})

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventTicketUpdated, projectID, id, "", map[string]interface{}{
			"action": "deleted",
		})
		s.wsHub.Broadcast(event)
	}
}

// CloseTicketRequest represents the request body for closing a ticket
type CloseTicketRequest struct {
	Reason string `json:"reason,omitempty"`
}

// handleCloseTicket closes a ticket
func (s *Server) handleCloseTicket(w http.ResponseWriter, r *http.Request) {
	ticketRepo := s.ticketRepo()
	depRepo := s.depRepo()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)

	var req CloseTicketRequest
	if r.ContentLength > 0 {
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	if err := ticketRepo.Close(projectID, id, req.Reason); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Fetch updated ticket
	closed, err := ticketRepo.Get(projectID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, closed)

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventTicketUpdated, projectID, id, "", map[string]interface{}{
			"status": string(closed.Status),
		})
		s.wsHub.Broadcast(event)

		// Broadcast updates for tickets that were blocked by this ticket
		blocked, err := depRepo.GetBlocked(projectID, id)
		if err != nil {
			logger.Error(context.Background(), "failed to get blocked tickets after close", "ticket_id", id, "error", err)
		} else {
			for _, dep := range blocked {
				evt := ws.NewEvent(ws.EventTicketUpdated, projectID, dep.IssueID, "", map[string]interface{}{
					"unblocked_by": id,
				})
				s.wsHub.Broadcast(evt)
			}
		}
	}

	// Best-effort: auto-close parent epic if all children are now closed
	ticketService := service.NewTicketService(s.pool, s.clock)
	epic, err := ticketService.TryCloseParentEpic(projectID, id)
	if err != nil {
		logger.Error(context.Background(), "failed to auto-close parent epic", "ticket_id", id, "error", err)
	} else if epic != nil && s.wsHub != nil {
		s.wsHub.Broadcast(ws.NewEvent(ws.EventTicketUpdated, projectID, epic.ID, "", map[string]interface{}{"status": "closed"}))
	}
}

// handleReopenTicket reopens a closed ticket
func (s *Server) handleReopenTicket(w http.ResponseWriter, r *http.Request) {
	ticketRepo := s.ticketRepo()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)

	if err := ticketRepo.Reopen(projectID, id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Fetch updated ticket
	reopened, err := ticketRepo.Get(projectID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, reopened)

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventTicketUpdated, projectID, id, "", map[string]interface{}{
			"status": string(reopened.Status),
		})
		s.wsHub.Broadcast(event)
	}
}
