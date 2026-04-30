package api

import (
	"net/http"
	"strings"

	"be/internal/repo"
)

// handleGetSessionPrompt returns the rendered user prompt and system prompt
// stored on the agent_sessions row. Returns 200 with {prompt, system_prompt},
// 204 if both are empty/NULL, 404 if the session is not found.
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

	prompt := ""
	if session.Prompt.Valid {
		prompt = session.Prompt.String
	}
	systemPrompt := ""
	if session.SystemPrompt.Valid {
		systemPrompt = session.SystemPrompt.String
	}
	if prompt == "" && systemPrompt == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"prompt":        prompt,
		"system_prompt": systemPrompt,
	})
}
