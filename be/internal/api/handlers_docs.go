package api

import (
	"net/http"

	"be/internal/static"
)

// handleGetAgentManual serves the embedded agent_manual.md content as JSON.
func (s *Server) handleGetAgentManual(w http.ResponseWriter, r *http.Request) {
	content := static.AgentManual()
	if content == "" {
		writeError(w, http.StatusNotFound, "agent manual not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"content": content,
		"title":   "Agent Documentation",
	})
}
