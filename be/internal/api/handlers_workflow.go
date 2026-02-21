package api

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	workflowSvc := service.NewWorkflowService(pool, s.clock)

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
	Workflow string `json:"workflow"`
}

// handleUpdateWorkflow broadcasts a workflow update event.
func (s *Server) handleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
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

	// Broadcast workflow update
	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventWorkflowUpdated, projectID, id, req.Workflow, nil)
		s.wsHub.Broadcast(event)
	}

	// Return the current workflow state
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

	agentSessionRepo := repo.NewAgentSessionRepo(database, s.clock)
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
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.clock)
	findings := make(map[string]interface{})

	instances, _ := wfiRepo.ListByTicket(projectID, id)
	for _, wi := range instances {
		workflowSvc := service.NewWorkflowService(pool, s.clock)
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
	agentSvc := service.NewAgentService(pool, s.clock)

	// Parse pagination params
	limit := 0
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

// handleGetRecentAgents returns recent agent sessions across all projects
func (s *Server) handleGetRecentAgents(w http.ResponseWriter, r *http.Request) {
	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	agentSessionRepo := repo.NewAgentSessionRepo(database, s.clock)
	projectRepo := repo.NewProjectRepo(database, s.clock)

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

// handleGetRunningAgents returns currently running agent sessions across all projects.
// No X-Project header required — this is a global endpoint.
func (s *Server) handleGetRunningAgents(w http.ResponseWriter, r *http.Request) {
	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	agentSessionRepo := repo.NewAgentSessionRepo(database, s.clock)
	projectRepo := repo.NewProjectRepo(database, s.clock)

	// Parse limit from query param (default 50, max 100)
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
			if limit > 100 {
				limit = 100
			}
		}
	}

	sessions, err := agentSessionRepo.GetRunning(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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

	now := s.clock.Now()
	agents := make([]map[string]interface{}, 0, len(sessions))
	for _, sess := range sessions {
		var elapsedSec float64
		if sess.StartedAt.Valid {
			if t, err := time.Parse(time.RFC3339Nano, sess.StartedAt.String); err == nil {
				elapsedSec = math.Round(now.Sub(t).Seconds())
			}
		}
		var modelID string
		if sess.ModelID.Valid {
			modelID = sess.ModelID.String
		}
		agents = append(agents, map[string]interface{}{
			"session_id":   sess.ID,
			"project_id":   sess.ProjectID,
			"project_name": projectMap[sess.ProjectID],
			"ticket_id":    sess.TicketID,
			"workflow_id":  sess.Workflow,
			"agent_type":   sess.AgentType,
			"model_id":     modelID,
			"phase":        sess.Phase,
			"started_at":   sess.StartedAt.String,
			"elapsed_sec":  elapsedSec,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"agents": agents,
		"count":  len(agents),
	})
}
