package api

import (
	"net/http"
	"os"
	"path/filepath"
)

// handleGetAgentManual serves the agent_manual.md content as JSON.
func (s *Server) handleGetAgentManual(w http.ResponseWriter, r *http.Request) {
	filePath := filepath.Join(s.dataPath, "agent_manual.md")
	content, err := os.ReadFile(filePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent manual not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"content": string(content),
		"title":   "Agent Documentation",
	})
}
