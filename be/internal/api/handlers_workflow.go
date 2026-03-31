package api

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// handleGetWorkflow returns the workflow state for a ticket from workflow_instances + agent_sessions.
// Keys all_workflows by instance_id (like project workflow handler).
func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)
	workflowSvc := s.workflowService()

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

	// Key all_workflows by instance_id. Build deduplicated workflow names list.
	allWorkflows := make(map[string]interface{})
	workflowNamesSet := make(map[string]bool)
	var firstInstanceID string
	for _, wi := range instances {
		workflowNamesSet[wi.WorkflowID] = true
		state, err := workflowSvc.GetStatusByInstance(wi)
		if err != nil {
			continue
		}
		allWorkflows[wi.ID] = state
		if firstInstanceID == "" {
			firstInstanceID = wi.ID
		}
	}

	workflowNames := make([]string, 0, len(workflowNamesSet))
	for name := range workflowNamesSet {
		workflowNames = append(workflowNames, name)
	}

	// Select state for display
	requestedWorkflow := r.URL.Query().Get("workflow")
	requestedInstance := r.URL.Query().Get("instance_id")
	var selectedState interface{}
	if requestedInstance != "" {
		selectedState = allWorkflows[requestedInstance]
	}
	if selectedState == nil && requestedWorkflow != "" {
		// Find first instance matching the requested workflow
		for _, wi := range instances {
			if wi.WorkflowID == requestedWorkflow {
				selectedState = allWorkflows[wi.ID]
				break
			}
		}
	}
	if selectedState == nil && firstInstanceID != "" {
		selectedState = allWorkflows[firstInstanceID]
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

// handleGetAgentSessions returns agent sessions for a ticket with findings from DB.
// Accepts optional instance_id query param to filter sessions to a specific instance.
func (s *Server) handleGetAgentSessions(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)
	phase := r.URL.Query().Get("phase")
	instanceID := r.URL.Query().Get("instance_id")

	sessions, err := s.agentSessionRepo().GetByTicket(projectID, id, phase)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if sessions == nil {
		sessions = []*model.AgentSession{}
	}

	// Filter sessions by instance_id if provided
	if instanceID != "" {
		filtered := make([]*model.AgentSession, 0)
		for _, sess := range sessions {
			if sess.WorkflowInstanceID == instanceID {
				filtered = append(filtered, sess)
			}
		}
		sessions = filtered
	}

	// Build findings from workflow_instances + agent_sessions
	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	findings := make(map[string]interface{})

	if instanceID != "" {
		// Only build findings for the specific instance
		if wi, err := wfiRepo.Get(instanceID); err == nil {
			workflowSvc := s.workflowService()
			findings = workflowSvc.BuildCombinedFindings(wi)
		}
	} else {
		instances, _ := wfiRepo.ListByTicket(projectID, id)
		workflowSvc := s.workflowService()
		for _, wi := range instances {
			combined := workflowSvc.BuildCombinedFindings(wi)
			for k, v := range combined {
				findings[k] = v
			}
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
	sessionID := extractID(r)

	agentSvc := service.NewAgentService(s.pool, s.clock)

	// Parse pagination and filter params
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
	category := r.URL.Query().Get("category")

	messages, total, err := agentSvc.GetSessionMessages(sessionID, limit, offset, category)
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
	agentSessionRepo := s.agentSessionRepo()
	projectRepo := s.projectRepo()

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
	agentSessionRepo := s.agentSessionRepo()
	projectRepo := s.projectRepo()

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
