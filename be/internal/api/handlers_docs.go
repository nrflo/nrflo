package api

import (
	"net/http"

	"be/internal/static"
)

var allowedDocKinds = map[string]string{
	"common":          "Common",
	"cli":             "CLI Agents",
	"python":          "Python Agents",
	"api":             "API Agents",
	"local-providers": "Local Providers",
	"mcp-external":    "External MCP",
}

// handleGetAgentManual serves embedded documentation as JSON.
func (s *Server) handleGetAgentManual(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "common"
	}
	title, ok := allowedDocKinds[kind]
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown kind")
		return
	}
	content := static.Manual(kind)
	if content == "" {
		writeError(w, http.StatusNotFound, "documentation not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"content": content,
		"title":   title,
	})
}
