package api

import (
	"net/http"
	"strings"

	"be/internal/handoff"
	"be/internal/repo"
)

// handleGetHandoffDigest returns the current autonomous-refinery slot digest
// (content/version/fold_count/updated_at) for a session's
// (workflow_instance_id, node_id) slot. Unlike handleGetContextLedger (an
// in-memory, running-session-only snapshot), this reads the durable
// refinery_autonomous_digests row via repo, so it also serves finished
// sessions.
// GET /api/v1/sessions/{id}/handoff-digest
func (s *Server) handleGetHandoffDigest(w http.ResponseWriter, r *http.Request) {
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

	if projectID := getProjectID(r); projectID != "" && !strings.EqualFold(session.ProjectID, projectID) {
		writeError(w, http.StatusForbidden, "session does not belong to this project")
		return
	}

	digestRepo := repo.NewRefineryDigestRepo(s.pool, s.clock)
	digest, err := digestRepo.GetSlot(session.WorkflowInstanceID, session.NodeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load handoff digest")
		return
	}

	digestContent := ""
	if digest != nil {
		digestContent = digest.Content
	}
	composed := handoff.Compose(r.Context(), s.pool, s.clock, sessionID, digestContent)

	if composed == "" && digest == nil {
		writeError(w, http.StatusNotFound, "no handoff digest for this session")
		return
	}

	if digest != nil {
		digest.Content = composed
		writeJSON(w, http.StatusOK, digest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"workflow_instance_id": session.WorkflowInstanceID,
		"node_id":              session.NodeID,
		"project_id":           session.ProjectID,
		"version":              0,
		"content":              composed,
		"fold_count":           0,
		"updated_at":           s.clock.Now(),
	})
}
