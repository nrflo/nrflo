package api

import (
	"fmt"
	"net/http"
	"strings"

	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/ws"
)

// handleRunWorkflow starts an orchestrated workflow run.
// POST /api/v1/tickets/:id/workflow/run
func (s *Server) handleRunWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}

	ticketID := extractID(r)
	if ticketID == "" {
		writeError(w, http.StatusBadRequest, "ticket ID required")
		return
	}

	if s.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "orchestrator not available")
		return
	}

	var body struct {
		Workflow     string `json:"workflow"`
		Instructions string `json:"instructions"`
		Interactive  bool   `json:"interactive"`
		PlanMode     bool   `json:"plan_mode"`
		Force        bool   `json:"force"`
	}
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

	if err := s.ticketService().ValidateRunnable(projectID, ticketID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusConflict, err.Error())
		}
		return
	}

	result, err := s.orchestrator.Start(r.Context(), orchestrator.RunRequest{
		ProjectID:    projectID,
		TicketID:     ticketID,
		WorkflowName: body.Workflow,
		Instructions: body.Instructions,
		Interactive:  body.Interactive,
		PlanMode:     body.PlanMode,
		Force:        body.Force,
	})
	if err != nil {
		// Check if it's a "already running" or concurrent ticket workflow error
		if s.orchestrator.IsRunning(projectID, ticketID, body.Workflow) ||
			strings.Contains(err.Error(), "concurrent ticket workflows") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleRestartAgent triggers a manual agent restart (context save + relaunch).
// POST /api/v1/tickets/:id/workflow/restart
func (s *Server) handleRestartAgent(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}

	ticketID := extractID(r)
	if ticketID == "" {
		writeError(w, http.StatusBadRequest, "ticket ID required")
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

	err := s.orchestrator.RestartAgent(projectID, ticketID, body.Workflow, body.SessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "restarting"})
}

// handleRetryFailedAgent retries a failed workflow from the failed layer.
// POST /api/v1/tickets/:id/workflow/retry-failed
func (s *Server) handleRetryFailedAgent(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}

	ticketID := extractID(r)
	if ticketID == "" {
		writeError(w, http.StatusBadRequest, "ticket ID required")
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

	if err := s.ticketService().ValidateRunnable(projectID, ticketID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusConflict, err.Error())
		}
		return
	}

	err := s.orchestrator.RetryFailedAgent(r.Context(), projectID, ticketID, body.Workflow, body.SessionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "retrying"})
}

// handleTakeControl initiates a take-control flow: kills the agent and returns the session ID.
// POST /api/v1/tickets/:id/workflow/take-control
func (s *Server) handleTakeControl(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}

	ticketID := extractID(r)
	if ticketID == "" {
		writeError(w, http.StatusBadRequest, "ticket ID required")
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

	sessionID, err := s.orchestrator.TakeControl(projectID, ticketID, body.Workflow, body.SessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "interactive", "session_id": sessionID})
}

// handleResumeSession sets a finished agent session to user_interactive status
// without requiring a running orchestration. Reuses the existing PTY handler.
// POST /api/v1/tickets/:id/workflow/resume-session
func (s *Server) handleResumeSession(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
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

	// Look up workflow name for the broadcast event.
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

// handleExitInteractive signals that the interactive session has ended.
// POST /api/v1/tickets/:id/workflow/exit-interactive
func (s *Server) handleExitInteractive(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}

	ticketID := extractID(r)
	if ticketID == "" {
		writeError(w, http.StatusBadRequest, "ticket ID required")
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
	_ = ticketID

	err := s.orchestrator.CompleteInteractive(body.SessionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

// handleStopWorkflow stops a running orchestrated workflow.
// POST /api/v1/tickets/:id/workflow/stop
func (s *Server) handleStopWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}

	ticketID := extractID(r)
	if ticketID == "" {
		writeError(w, http.StatusBadRequest, "ticket ID required")
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
	// Body is optional
	readJSON(r, &body)

	err := s.orchestrator.StopByTicket(projectID, ticketID, body.Workflow, body.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}

// validateResumeSession checks that a session is eligible for resume:
// must be a Claude CLI agent in a terminal state.
func validateResumeSession(session *model.AgentSession) error {
	// Check model_id is valid and indicates a Claude CLI agent.
	if !session.ModelID.Valid || session.ModelID.String == "" {
		return fmt.Errorf("session has no model_id, cannot determine CLI type")
	}
	cliName := session.ModelID.String
	if idx := strings.Index(cliName, ":"); idx >= 0 {
		cliName = cliName[:idx]
	}
	if cliName != "claude" {
		return fmt.Errorf("session CLI %q does not support resume (only Claude CLI supports resume)", cliName)
	}

	// Check session is in a terminal state.
	switch session.Status {
	case model.AgentSessionCompleted, model.AgentSessionFailed, model.AgentSessionTimeout,
		model.AgentSessionInteractiveCompleted, model.AgentSessionSkipped:
		// OK
	default:
		return fmt.Errorf("session status is %s, expected a terminal state (completed, failed, timeout, interactive_completed, skipped)", session.Status)
	}
	return nil
}
