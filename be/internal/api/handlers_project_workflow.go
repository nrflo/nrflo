package api

import (
	"context"
	"net/http"

	"be/internal/db"
	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
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

	result, err := s.orchestrator.Start(context.Background(), orchestrator.RunRequest{
		ProjectID:    projectID,
		WorkflowName: body.Workflow,
		Instructions: body.Instructions,
		ScopeType:    "project",
	})
	if err != nil {
		if s.orchestrator.IsProjectRunning(projectID, body.Workflow) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
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
		Workflow string `json:"workflow"`
	}
	readJSON(r, &body)

	err := s.orchestrator.StopByProject(projectID, body.Workflow)
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

	err := s.orchestrator.RestartProjectAgent(projectID, body.Workflow, body.SessionID)
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

	err := s.orchestrator.RetryFailedProjectAgent(context.Background(), projectID, body.Workflow, body.SessionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "retrying"})
}

// handleGetProjectWorkflow returns the workflow state for a project-scoped workflow.
// GET /api/v1/projects/{id}/workflow
func (s *Server) handleGetProjectWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}

	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	workflowSvc := service.NewWorkflowService(pool)

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

	workflowNames := make([]string, 0, len(instances))
	allWorkflows := make(map[string]interface{})
	for _, wi := range instances {
		workflowNames = append(workflowNames, wi.WorkflowID)
		state, err := workflowSvc.GetStatusByInstance(wi)
		if err != nil {
			continue
		}
		allWorkflows[wi.WorkflowID] = state
	}

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

	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	phase := r.URL.Query().Get("phase")

	agentSessionRepo := repo.NewAgentSessionRepo(database)
	sessions, err := agentSessionRepo.GetByProjectScope(projectID, phase)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if sessions == nil {
		sessions = []*model.AgentSession{}
	}

	// Build findings from project-scoped workflow instances
	pool := db.WrapAsPool(database)
	workflowSvc := service.NewWorkflowService(pool)
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
