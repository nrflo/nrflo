package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"nrworkflow/internal/id"
	"nrworkflow/internal/model"
	"nrworkflow/internal/repo"
)

// handleListProjects returns all projects
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	_, _, _, projectRepo, database, err := s.getAllRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	projects, err := projectRepo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if projects == nil {
		projects = []*model.Project{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"projects": projects,
	})
}

// CreateProjectRequest represents the request body for creating a project
type CreateProjectRequest struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	RootPath        string `json:"root_path,omitempty"`
	DefaultWorkflow string `json:"default_workflow,omitempty"`
}

// handleCreateProject creates a new project
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	_, _, _, projectRepo, database, err := s.getAllRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	var req CreateProjectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if req.Name == "" {
		req.Name = req.ID
	}

	project := &model.Project{
		ID:   req.ID,
		Name: req.Name,
	}

	if req.RootPath != "" {
		project.RootPath = sql.NullString{String: req.RootPath, Valid: true}
	}
	if req.DefaultWorkflow != "" {
		project.DefaultWorkflow = sql.NullString{String: req.DefaultWorkflow, Valid: true}
	}

	if err := projectRepo.Create(project); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	created, err := projectRepo.Get(req.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// handleGetProject returns a single project by ID
func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	_, _, _, projectRepo, database, err := s.getAllRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	id := extractID(r)
	project, err := projectRepo.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, project)
}

// handleDeleteProject deletes a project
func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	_, _, _, projectRepo, database, err := s.getAllRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	id := extractID(r)
	if err := projectRepo.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "project deleted"})
}

// UpdateProjectRequest represents the request body for updating a project
type UpdateProjectRequest struct {
	Name            *string `json:"name,omitempty"`
	RootPath        *string `json:"root_path,omitempty"`
	DefaultWorkflow *string `json:"default_workflow,omitempty"`
}

// handleUpdateProject updates a project
func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	_, _, _, projectRepo, database, err := s.getAllRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	id := extractID(r)

	var req UpdateProjectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fields := &repo.ProjectUpdateFields{
		Name:            req.Name,
		RootPath:        req.RootPath,
		DefaultWorkflow: req.DefaultWorkflow,
	}

	if err := projectRepo.Update(id, fields); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	updated, err := projectRepo.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// handleListTickets returns tickets with optional filters
func (s *Server) handleListTickets(w http.ResponseWriter, r *http.Request) {
	ticketRepo, _, database, err := s.getRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required (use X-Project header or ?project= query param)")
		return
	}

	filter := &repo.ListFilter{
		ProjectID: projectID,
		Status:    r.URL.Query().Get("status"),
		IssueType: r.URL.Query().Get("type"),
	}

	tickets, err := ticketRepo.List(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return empty array instead of null
	if tickets == nil {
		tickets = []*model.Ticket{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tickets": tickets,
	})
}

// CreateTicketRequest represents the request body for creating a ticket
type CreateTicketRequest struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Priority    int    `json:"priority,omitempty"`
	IssueType   string `json:"issue_type,omitempty"`
	CreatedBy   string `json:"created_by"`
}

// handleCreateTicket creates a new ticket
func (s *Server) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	ticketRepo, _, database, err := s.getRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

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
}

// handleGetTicket returns a single ticket by ID
func (s *Server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	ticketRepo, depRepo, database, err := s.getRepos(r)
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

	// Return ticket with dependencies
	response := struct {
		*model.Ticket
		Blockers []*model.Dependency `json:"blockers"`
		Blocks   []*model.Dependency `json:"blocks"`
	}{
		Ticket:   ticket,
		Blockers: blockers,
		Blocks:   blocked,
	}

	if response.Blockers == nil {
		response.Blockers = []*model.Dependency{}
	}
	if response.Blocks == nil {
		response.Blocks = []*model.Dependency{}
	}

	writeJSON(w, http.StatusOK, response)
}

// UpdateTicketRequest represents the request body for updating a ticket
type UpdateTicketRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
	Priority    *int    `json:"priority,omitempty"`
	IssueType   *string `json:"issue_type,omitempty"`
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
		Title:       req.Title,
		Description: req.Description,
		Status:      req.Status,
		Priority:    req.Priority,
		IssueType:   req.IssueType,
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
}

// handleSearch performs FTS5 search
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
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

	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	tickets, err := ticketRepo.Search(projectID, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if tickets == nil {
		tickets = []*model.Ticket{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tickets": tickets,
		"query":   query,
	})
}

// handleStatus returns dashboard summary
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
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

	// Get pending tickets with blocked info
	pending, err := ticketRepo.GetPendingWithBlockedInfo(projectID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Get recently closed
	closed, err := ticketRepo.GetRecentlyClosed(projectID, 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Count by status
	allTickets, err := ticketRepo.List(&repo.ListFilter{ProjectID: projectID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	counts := map[string]int{
		"open":        0,
		"in_progress": 0,
		"closed":      0,
		"total":       len(allTickets),
	}
	for _, t := range allTickets {
		counts[string(t.Status)]++
	}

	// Count ready (not blocked)
	readyCount := 0
	for _, p := range pending {
		if !p.IsBlocked {
			readyCount++
		}
	}

	statusLimit := r.URL.Query().Get("limit")
	limit := 10
	if statusLimit != "" {
		if l, err := strconv.Atoi(statusLimit); err == nil && l > 0 {
			limit = l
		}
	}

	// Trim pending to limit
	if len(pending) > limit {
		pending = pending[:limit]
	}

	if pending == nil {
		pending = []*repo.PendingTicket{}
	}
	if closed == nil {
		closed = []*model.Ticket{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"counts":          counts,
		"ready_count":     readyCount,
		"pending_tickets": pending,
		"recent_closed":   closed,
	})
}

// AddDependencyRequest represents the request body for adding a dependency
type AddDependencyRequest struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
}

// handleAddDependency adds a dependency between tickets
func (s *Server) handleAddDependency(w http.ResponseWriter, r *http.Request) {
	_, depRepo, database, err := s.getRepos(r)
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

	var req AddDependencyRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.IssueID == "" || req.DependsOnID == "" {
		writeError(w, http.StatusBadRequest, "issue_id and depends_on_id are required")
		return
	}

	dep := &model.Dependency{
		ProjectID:   projectID,
		IssueID:     req.IssueID,
		DependsOnID: req.DependsOnID,
		Type:        "blocks",
		CreatedBy:   "api",
	}

	if err := depRepo.Create(dep); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "dependency added"})
}

// RemoveDependencyRequest represents the request body for removing a dependency
type RemoveDependencyRequest struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
}

// handleRemoveDependency removes a dependency between tickets
func (s *Server) handleRemoveDependency(w http.ResponseWriter, r *http.Request) {
	_, depRepo, database, err := s.getRepos(r)
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

	var req RemoveDependencyRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.IssueID == "" || req.DependsOnID == "" {
		writeError(w, http.StatusBadRequest, "issue_id and depends_on_id are required")
		return
	}

	if err := depRepo.Delete(projectID, req.IssueID, req.DependsOnID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "dependency removed"})
}

// handleGetDependencies returns dependencies for a ticket
func (s *Server) handleGetDependencies(w http.ResponseWriter, r *http.Request) {
	_, depRepo, database, err := s.getRepos(r)
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

	if blockers == nil {
		blockers = []*model.Dependency{}
	}
	if blocked == nil {
		blocked = []*model.Dependency{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"blockers": blockers,
		"blocks":   blocked,
	})
}
