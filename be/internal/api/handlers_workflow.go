package api

import (
	"net/http"
	"strconv"
	"strings"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// handleGetWorkflow returns the workflow state for a ticket from workflow_instances + agent_sessions
func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	database, err := s.getDatabase()
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
	pool := db.WrapAsPool(database)
	workflowSvc := service.NewWorkflowService(pool)

	// List all workflow instances for this ticket
	instances, err := workflowSvc.ListWorkflowInstances(projectID, id)
	if err != nil || len(instances) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ticket_id":    id,
			"has_workflow": false,
			"state":        emptyWorkflowState(),
			"workflows":    []string{},
		})
		return
	}

	// Build state for each workflow
	workflowNames := make([]string, 0, len(instances))
	allWorkflows := make(map[string]interface{})
	for _, wi := range instances {
		workflowNames = append(workflowNames, wi.WorkflowID)
		state, err := workflowSvc.GetStatus(projectID, id, &types.WorkflowGetRequest{Workflow: wi.WorkflowID})
		if err != nil {
			continue
		}
		allWorkflows[wi.WorkflowID] = state
	}

	// Select the requested workflow or default to first
	requestedWorkflow := r.URL.Query().Get("workflow")
	var selectedState interface{}
	if requestedWorkflow != "" {
		selectedState = allWorkflows[requestedWorkflow]
	}
	if selectedState == nil && len(workflowNames) > 0 {
		selectedState = allWorkflows[workflowNames[0]]
	}
	if selectedState == nil {
		selectedState = emptyWorkflowState()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ticket_id":     id,
		"has_workflow":  len(workflowNames) > 0,
		"state":         selectedState,
		"workflows":     workflowNames,
		"all_workflows": allWorkflows,
	})
}

// emptyWorkflowState returns an empty workflow state for API responses
func emptyWorkflowState() map[string]interface{} {
	return map[string]interface{}{
		"phases":        map[string]interface{}{},
		"active_agents": map[string]interface{}{},
	}
}

// UpdateWorkflowRequest represents the request to update workflow state
type UpdateWorkflowRequest struct {
	Workflow      string `json:"workflow"`
	CurrentPhase  *string `json:"current_phase,omitempty"`
	Category      *string `json:"category,omitempty"`
}

// handleUpdateWorkflow updates the workflow instance for a ticket
func (s *Server) handleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	database, err := s.getDatabase()
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

	var req UpdateWorkflowRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)

	// Resolve workflow name
	workflowName := req.Workflow
	if workflowName == "" {
		instances, err := wfiRepo.ListByTicket(projectID, id)
		if err != nil || len(instances) == 0 {
			writeError(w, http.StatusNotFound, "no workflow found on ticket")
			return
		}
		workflowName = instances[0].WorkflowID
	}

	wi, err := wfiRepo.GetByTicketAndWorkflow(projectID, id, workflowName)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Apply updates
	if req.CurrentPhase != nil {
		wfiRepo.UpdateCurrentPhase(wi.ID, *req.CurrentPhase)
	}
	if req.Category != nil {
		wfiRepo.UpdateCategory(wi.ID, *req.Category)
	}

	// Broadcast workflow update
	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventWorkflowUpdated, projectID, id, workflowName, nil)
		s.wsHub.Broadcast(event)
	}

	// Return the updated workflow
	s.handleGetWorkflow(w, r)
}

// handleGetAgentSessions returns agent sessions for a ticket with findings from DB
func (s *Server) handleGetAgentSessions(w http.ResponseWriter, r *http.Request) {
	database, err := s.getDatabase()
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
	phase := r.URL.Query().Get("phase")

	agentSessionRepo := repo.NewAgentSessionRepo(database)
	sessions, err := agentSessionRepo.GetByTicket(projectID, id, phase)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if sessions == nil {
		sessions = []*model.AgentSession{}
	}

	// Build findings from workflow_instances + agent_sessions
	pool := db.WrapAsPool(database)
	wfiRepo := repo.NewWorkflowInstanceRepo(pool)
	findings := make(map[string]interface{})

	instances, _ := wfiRepo.ListByTicket(projectID, id)
	for _, wi := range instances {
		workflowSvc := service.NewWorkflowService(pool)
		combined := workflowSvc.BuildCombinedFindings(wi)
		for k, v := range combined {
			findings[k] = v
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ticket_id": id,
		"sessions":  sessions,
		"findings":  findings,
	})
}

// handleGetSessionMessages returns paginated messages for an agent session
func (s *Server) handleGetSessionMessages(w http.ResponseWriter, r *http.Request) {
	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	sessionID := extractID(r)

	pool := db.WrapAsPool(database)
	agentSvc := service.NewAgentService(pool)

	// Parse pagination params
	limit := 100
	offset := 0
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	messages, total, err := agentSvc.GetSessionMessages(sessionID, limit, offset)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_id": sessionID,
		"messages":   messages,
		"total":      total,
	})
}

// handleGetSessionRawOutput returns raw stdout/stderr output for an agent session
func (s *Server) handleGetSessionRawOutput(w http.ResponseWriter, r *http.Request) {
	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	sessionID := extractID(r)

	pool := db.WrapAsPool(database)
	agentSvc := service.NewAgentService(pool)

	rawOutput, err := agentSvc.GetSessionRawOutput(sessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_id": sessionID,
		"raw_output": rawOutput,
	})
}

// handleGetRecentAgents returns recent agent sessions across all projects
func (s *Server) handleGetRecentAgents(w http.ResponseWriter, r *http.Request) {
	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	agentSessionRepo := repo.NewAgentSessionRepo(database)
	projectRepo := repo.NewProjectRepo(database)

	// Parse limit from query param (default 10, max 50)
	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
			if limit > 50 {
				limit = 50
			}
		}
	}

	sessions, err := agentSessionRepo.GetRecent(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if sessions == nil {
		sessions = []*model.AgentSession{}
	}

	// Get project names
	projects, err := projectRepo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	projectMap := make(map[string]string)
	for _, p := range projects {
		projectMap[p.ID] = p.Name
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": sessions,
		"projects": projectMap,
	})
}
