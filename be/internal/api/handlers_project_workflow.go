package api

import (
	"net/http"
	"strings"

	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/types"
	"be/internal/ws"
)

// handleRunProjectWorkflow starts an orchestrated project-scoped workflow run.
// POST /api/v1/projects/{id}/workflow/run
func (s *Server) handleRunProjectWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}

	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not available")
		return
	}

	var body types.ProjectWorkflowRunRequest
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Workflow == "" {
		writeError(w, http.StatusBadRequest, "workflow name is required")
		return
	}

	if body.Interactive && body.PlanMode {
		writeError(w, http.StatusBadRequest, "interactive and plan_mode are mutually exclusive")
		return
	}

	result, err := s.orchestrator.Start(r.Context(), orchestrator.RunRequest{
		ProjectID:    projectID,
		WorkflowName: body.Workflow,
		Instructions: body.Instructions,
		ScopeType:    "project",
		Interactive:  body.Interactive,
		PlanMode:     body.PlanMode,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleStopProjectWorkflow stops a running project-scoped workflow.
// POST /api/v1/projects/{id}/workflow/stop
func (s *Server) handleStopProjectWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}

	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not available")
		return
	}

	var body struct {
		Workflow   string `json:"workflow"`
		InstanceID string `json:"instance_id"`
	}
	readJSON(r, &body)

	err := s.orchestrator.StopByProject(projectID, body.Workflow, body.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}

// handleRestartProjectAgent triggers a manual agent restart for a project-scoped workflow.
// POST /api/v1/projects/{id}/workflow/restart
func (s *Server) handleRestartProjectAgent(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}

	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not available")
		return
	}

	var body struct {
		Workflow   string `json:"workflow"`
		SessionID  string `json:"session_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Workflow == "" {
		writeError(w, http.StatusBadRequest, "workflow name is required")
		return
	}
	if body.SessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	err := s.orchestrator.RestartProjectAgent(projectID, body.Workflow, body.SessionID, body.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "restarting"})
}

// handleRetryFailedProjectAgent retries a failed project-scoped workflow from the failed layer.
// POST /api/v1/projects/{id}/workflow/retry-failed
func (s *Server) handleRetryFailedProjectAgent(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}

	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not available")
		return
	}

	var body struct {
		Workflow   string `json:"workflow"`
		SessionID  string `json:"session_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Workflow == "" {
		writeError(w, http.StatusBadRequest, "workflow name is required")
		return
	}
	if body.SessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	err := s.orchestrator.RetryFailedProjectAgent(r.Context(), projectID, body.Workflow, body.SessionID, body.InstanceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "retrying"})
}

// handleTakeControlProject initiates a take-control flow for a project-scoped workflow.
// POST /api/v1/projects/{id}/workflow/take-control
func (s *Server) handleTakeControlProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}

	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not available")
		return
	}

	var body struct {
		Workflow   string `json:"workflow"`
		SessionID  string `json:"session_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Workflow == "" {
		writeError(w, http.StatusBadRequest, "workflow name is required")
		return
	}
	if body.SessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	sessionID, err := s.orchestrator.TakeControlProject(projectID, body.Workflow, body.SessionID, body.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "interactive", "session_id": sessionID})
}

// handleResumeSessionProject sets a finished project-scoped agent session to
// user_interactive status without requiring a running orchestration.
// POST /api/v1/projects/{id}/workflow/resume-session
func (s *Server) handleResumeSessionProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}

	var body struct {
		SessionID string `json:"session_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.SessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	asRepo := s.agentSessionRepo()
	session, err := asRepo.Get(body.SessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	if !strings.EqualFold(session.ProjectID, projectID) {
		writeError(w, http.StatusBadRequest, "session does not belong to this project")
		return
	}

	if err := validateResumeSession(session); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := asRepo.UpdateStatus(body.SessionID, model.AgentSessionUserInteractive); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	workflowName := ""
	if wfi, err := wfiRepo.Get(session.WorkflowInstanceID); err == nil {
		workflowName = wfi.WorkflowID
	}

	s.wsHub.Broadcast(ws.NewEvent(ws.EventAgentTakeControl, session.ProjectID, session.TicketID, workflowName, map[string]interface{}{
		"session_id": session.ID,
		"agent_type": session.AgentType,
		"model_id":   session.ModelID.String,
	}))

	writeJSON(w, http.StatusOK, map[string]string{"status": "interactive", "session_id": body.SessionID})
}

// handleExitInteractiveProject signals that the interactive session has ended for a project-scoped workflow.
// POST /api/v1/projects/{id}/workflow/exit-interactive
func (s *Server) handleExitInteractiveProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}

	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not available")
		return
	}

	var body struct {
		Workflow  string `json:"workflow"`
		SessionID string `json:"session_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Workflow == "" {
		writeError(w, http.StatusBadRequest, "workflow name is required")
		return
	}
	if body.SessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	_ = projectID // validated above; CompleteInteractive uses sessionID directly

	err := s.orchestrator.CompleteInteractive(body.SessionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

// handleGetProjectWorkflow returns the workflow state for a project-scoped workflow.
// GET /api/v1/projects/{id}/workflow
func (s *Server) handleGetProjectWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}

	workflowSvc := s.workflowService()

	instances, err := workflowSvc.ListProjectWorkflowInstances(projectID)
	if err != nil || len(instances) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"project_id":   projectID,
			"has_workflow": false,
			"state":        emptyWorkflowState(),
			"workflows":    []string{},
		})
		return
	}

	// Key all_workflows by instance_id instead of workflow name.
	// Build deduplicated workflow names list.
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
		"project_id":    projectID,
		"has_workflow":  len(workflowNames) > 0,
		"state":         selectedState,
		"workflows":     workflowNames,
		"all_workflows": allWorkflows,
	})
}

// handleGetProjectAgentSessions returns agent sessions for project-scoped workflows.
// GET /api/v1/projects/{id}/agents
func (s *Server) handleGetProjectAgentSessions(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}

	phase := r.URL.Query().Get("phase")

	sessions, err := s.agentSessionRepo().GetByProjectScope(projectID, phase)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if sessions == nil {
		sessions = []*model.AgentSession{}
	}

	// Build findings from project-scoped workflow instances
	workflowSvc := s.workflowService()
	findings := make(map[string]interface{})

	instances, _ := workflowSvc.ListProjectWorkflowInstances(projectID)
	for _, wi := range instances {
		combined := workflowSvc.BuildCombinedFindings(wi)
		for k, v := range combined {
			findings[k] = v
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"project_id": projectID,
		"sessions":   sessions,
		"findings":   findings,
	})
}
