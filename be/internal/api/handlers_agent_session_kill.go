package api

import (
	"net/http"
	"strings"

	"be/internal/model"
	"be/internal/repo"
)

// handleKillAgentSession sends a manual kill signal to a running agent session.
// POST /api/v1/agent-sessions/{id}/kill
func (s *Server) handleKillAgentSession(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required (use X-Project header or ?project= query param)")
		return
	}

	sessionID := extractID(r)
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID required")
		return
	}

	session, err := s.agentSessionRepo().Get(sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	if !strings.EqualFold(session.ProjectID, projectID) {
		writeError(w, http.StatusForbidden, "session does not belong to this project")
		return
	}

	if session.Status != model.AgentSessionRunning && session.Status != model.AgentSessionUserInteractive {
		writeError(w, http.StatusConflict, "not_alive")
		return
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	workflowName := ""
	if wfi, err := wfiRepo.Get(session.WorkflowInstanceID); err == nil {
		workflowName = wfi.WorkflowID
	}

	if err := s.orchestrator.RequestTerminalSignal(session.ProjectID, session.TicketID, workflowName, sessionID, "manual_kill"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":     "killed",
		"session_id": sessionID,
	})
}
