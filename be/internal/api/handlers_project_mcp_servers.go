package api

import (
	"encoding/json"
	"net/http"

	"be/internal/spawner"
)

// projectMCPServersPayload carries the external_mcp_servers project config:
// a map of server name → MCP server spec (spawner.ExternalMCPServer shape),
// or null when unset.
type projectMCPServersPayload struct {
	Servers json.RawMessage `json:"servers"`
}

func (s *Server) handleGetProjectMCPServers(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	raw, err := s.pool.GetProjectConfig(projectID, "external_mcp_servers")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := projectMCPServersPayload{Servers: json.RawMessage("null")}
	if raw != "" {
		resp.Servers = json.RawMessage(raw)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePutProjectMCPServers(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	var req projectMCPServersPayload
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	val := string(req.Servers)
	if val == "" || val == "null" || val == "{}" {
		val = ""
	} else if _, err := spawner.ParseExternalMCPServers(val); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.pool.SetProjectConfig(projectID, "external_mcp_servers", val); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := projectMCPServersPayload{Servers: json.RawMessage("null")}
	if val != "" {
		resp.Servers = json.RawMessage(val)
	}
	writeJSON(w, http.StatusOK, resp)
}
