package api

import (
	"context"
	"net/http"

	"be/internal/orchestrator"
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
		Category     string `json:"category"`
		Instructions string `json:"instructions"`
	}
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
		TicketID:     ticketID,
		WorkflowName: body.Workflow,
		Category:     body.Category,
		Instructions: body.Instructions,
	})
	if err != nil {
		// Check if it's a "already running" error
		if s.orchestrator.IsRunning(projectID, ticketID, body.Workflow) {
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

	err := s.orchestrator.RetryFailedAgent(context.Background(), projectID, ticketID, body.Workflow, body.SessionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "retrying"})
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
		Workflow string `json:"workflow"`
	}
	// Body is optional
	readJSON(r, &body)

	err := s.orchestrator.StopByTicket(projectID, ticketID, body.Workflow)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
}
