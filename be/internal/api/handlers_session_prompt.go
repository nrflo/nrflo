package api

import (
	"net/http"
	"strings"

	"be/internal/repo"
)

// handleGetSessionPrompt returns the prompt context for an agent session.
// Returns 200 with {prompt_context: string}, 204 if empty/NULL, 404 if not found.
func (s *Server) handleGetSessionPrompt(w http.ResponseWriter, r *http.Request) {
	sessionID := extractID(r)

	sessionRepo := repo.NewAgentSessionRepo(s.pool, s.clock)
	session, err := sessionRepo.Get(sessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	if !session.PromptContext.Valid || session.PromptContext.String == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"prompt_context": session.PromptContext.String,
	})
}
