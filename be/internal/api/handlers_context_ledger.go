package api

import (
	"net/http"
	"strings"

	"be/internal/spawner"
)

// handleGetContextLedger returns a read-only snapshot of a session's
// in-memory context ledger (debug endpoint — the ledger is dropped when the
// session ends, so this only serves running/just-finished sessions).
// GET /api/v1/sessions/{id}/context-ledger
func (s *Server) handleGetContextLedger(w http.ResponseWriter, r *http.Request) {
	sessionID := extractID(r)
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID required")
		return
	}

	if projectID := getProjectID(r); projectID != "" {
		session, err := s.agentSessionRepo().Get(sessionID)
		if err != nil {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		if !strings.EqualFold(session.ProjectID, projectID) {
			writeError(w, http.StatusForbidden, "session does not belong to this project")
			return
		}
	}

	snapshot, ok := spawner.LedgerSnapshot(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "no context ledger for this session")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}
