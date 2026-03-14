package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"be/internal/id"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// handleListTickets returns tickets with optional filters
func (s *Server) handleListTickets(w http.ResponseWriter, r *http.Request) {
	ticketRepo := s.ticketRepo()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required (use X-Project header or ?project= query param)")
		return
	}

	status := r.URL.Query().Get("status")
	filter := &repo.ListFilter{
		ProjectID: projectID,
		Status:    status,
		IssueType: r.URL.Query().Get("type"),
	}
	if status == "blocked" {
		filter.BlockedOnly = true
		filter.Status = ""
	}

	tickets, err := ticketRepo.ListWithBlockedInfo(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if tickets == nil {
		tickets = []*repo.PendingTicket{}
	}

	// Enrich with workflow progress (derived from agent_sessions + workflow definition)
	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	instances, err := wfiRepo.ListActiveByProject(projectID)
	if err == nil {
		wfService := service.NewWorkflowService(s.pool, s.clock)
		progress := wfService.DeriveWorkflowProgress(instances)
		repo.AttachWorkflowProgress(tickets, progress)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tickets": tickets,
	})
}

// CreateTicketRequest represents the request body for creating a ticket
type CreateTicketRequest struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	Priority       int    `json:"priority,omitempty"`
	IssueType      string `json:"issue_type,omitempty"`
	CreatedBy      string `json:"created_by"`
	ParentTicketID string `json:"parent_ticket_id,omitempty"`
}

// handleCreateTicket creates a new ticket
func (s *Server) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	ticketRepo := s.ticketRepo()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required (use X-Project header or ?project= query param)")
		return
	}

	var req CreateTicketRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.CreatedBy == "" {
		writeError(w, http.StatusBadRequest, "created_by is required")
		return
	}

	// Use provided ID or auto-generate one
	ticketID := req.ID
	if ticketID == "" {
		gen := id.New(strings.ToUpper(projectID))
		var err error
		ticketID, err = gen.Generate()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate ticket ID")
			return
		}
	}

	// Set defaults
	if req.Priority == 0 {
		req.Priority = 2
	}
	if req.IssueType == "" {
		req.IssueType = "task"
	}

	ticket := &model.Ticket{
		ID:        ticketID,
		ProjectID: projectID,
		Title:     req.Title,
		Status:    model.StatusOpen,
		Priority:  req.Priority,
		IssueType: model.IssueType(req.IssueType),
		CreatedBy: req.CreatedBy,
	}

	if req.Description != "" {
		ticket.Description = sql.NullString{String: req.Description, Valid: true}
	}
	if req.ParentTicketID != "" {
		ticket.ParentTicketID = sql.NullString{String: strings.ToLower(req.ParentTicketID), Valid: true}
	}
	if err := ticketRepo.Create(ticket); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Fetch the created ticket to return full data
	created, err := ticketRepo.Get(projectID, ticketID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, created)

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventTicketUpdated, projectID, ticketID, "", map[string]interface{}{
			"status": string(model.StatusOpen),
			"action": "created",
		})
		s.wsHub.Broadcast(event)
	}
}

// handleGetTicket returns a single ticket by ID
func (s *Server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	ticketRepo := s.ticketRepo()
	depRepo := s.depRepo()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)
	ticket, err := ticketRepo.Get(projectID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Get dependencies
	blockers, err := depRepo.GetBlockers(projectID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	blocked, err := depRepo.GetBlocked(projectID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Fetch children for epic tickets
	var children []*model.Ticket
	if ticket.IssueType == model.IssueTypeEpic {
		children, err = ticketRepo.ListByParent(projectID, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Fetch parent ticket and siblings when ticket has a parent
	var parentTicket *model.Ticket
	var siblings []*model.Ticket
	if ticket.ParentTicketID.Valid {
		parent, err := ticketRepo.Get(projectID, ticket.ParentTicketID.String)
		if err == nil {
			parentTicket = parent
		}

		allSiblings, err := ticketRepo.ListByParent(projectID, ticket.ParentTicketID.String)
		if err == nil {
			// Filter out current ticket from siblings
			for _, s := range allSiblings {
				if !strings.EqualFold(s.ID, id) {
					siblings = append(siblings, s)
				}
			}
		}
	}

	// Build response by merging ticket JSON with dependency/children fields.
	// Cannot use struct embedding because model.Ticket has a custom MarshalJSON
	// that would shadow the outer struct's fields.
	ticketJSON, err := json.Marshal(ticket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var response map[string]interface{}
	json.Unmarshal(ticketJSON, &response)

	if blockers == nil {
		blockers = []*model.Dependency{}
	}
	if blocked == nil {
		blocked = []*model.Dependency{}
	}
	if children == nil {
		children = []*model.Ticket{}
	}
	if siblings == nil {
		siblings = []*model.Ticket{}
	}

	response["blockers"] = blockers
	response["blocks"] = blocked
	response["children"] = children
	response["parent_ticket"] = parentTicket
	response["siblings"] = siblings

	writeJSON(w, http.StatusOK, response)
}
