package api

import (
	"net/http"

	"be/internal/repo"
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
	ticketRepo, _, database, err := s.getRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

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
	ticketRepo, _, database, err := s.getRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

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
	ticketRepo, _, database, err := s.getRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

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
	}
}

// handleReopenTicket reopens a closed ticket
func (s *Server) handleReopenTicket(w http.ResponseWriter, r *http.Request) {
	ticketRepo, _, database, err := s.getRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

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
