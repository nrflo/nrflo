package api

import (
	"errors"
	"net/http"
	"strings"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// agentDefResponse carries the saved agent definition plus any non-blocking
// warnings (e.g. tools-CSV patterns that match no known tool).
type agentDefResponse struct {
	*model.AgentDefinition
	Warnings []string `json:"warnings,omitempty"`
}

// handleListAgentDefs returns all agent definitions for a workflow
func (s *Server) handleListAgentDefs(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	workflowID := r.PathValue("wid")

	svc := service.NewAgentDefinitionService(s.pool, s.clock, service.NewCLIModelService(s.pool, s.clock), service.NewAPIModelService(s.pool, s.clock), repo.NewPythonScriptRepo(s.pool, s.clock))

	defs, err := svc.ListAgentDefs(projectID, workflowID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, defs)
}

// handleCreateAgentDef creates a new agent definition
func (s *Server) handleCreateAgentDef(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	workflowID := r.PathValue("wid")

	var req types.AgentDefCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if req.Prompt == "" && req.ExecutionMode != "script" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	svc := service.NewAgentDefinitionService(s.pool, s.clock, service.NewCLIModelService(s.pool, s.clock), service.NewAPIModelService(s.pool, s.clock), repo.NewPythonScriptRepo(s.pool, s.clock))

	def, err := svc.CreateAgentDef(projectID, workflowID, &req)
	if err != nil {
		if errors.Is(err, service.ErrAPIModeDisabled) {
			writeError(w, http.StatusBadRequest, "api_mode_disabled")
			return
		}
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if isLayerValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventAgentDefCreated, projectID, "", "", map[string]interface{}{
			"workflow_id": workflowID,
			"agent_id":    req.ID,
		})
		s.wsHub.Broadcast(event)
	}

	warnings := toolsCSVWarnings(req.Tools, s.availableAgentTools(projectID))
	writeJSON(w, http.StatusCreated, agentDefResponse{AgentDefinition: def, Warnings: warnings})
}

// handleGetAgentDef returns a single agent definition
func (s *Server) handleGetAgentDef(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	workflowID := r.PathValue("wid")
	id := r.PathValue("id")

	svc := service.NewAgentDefinitionService(s.pool, s.clock, service.NewCLIModelService(s.pool, s.clock), service.NewAPIModelService(s.pool, s.clock), repo.NewPythonScriptRepo(s.pool, s.clock))

	def, err := svc.GetAgentDef(projectID, workflowID, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, def)
}

// handleUpdateAgentDef updates an agent definition
func (s *Server) handleUpdateAgentDef(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	workflowID := r.PathValue("wid")
	id := r.PathValue("id")

	var req types.AgentDefUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := service.NewAgentDefinitionService(s.pool, s.clock, service.NewCLIModelService(s.pool, s.clock), service.NewAPIModelService(s.pool, s.clock), repo.NewPythonScriptRepo(s.pool, s.clock))

	if err := svc.UpdateAgentDef(projectID, workflowID, id, &req); err != nil {
		if errors.Is(err, service.ErrAPIModeDisabled) {
			writeError(w, http.StatusBadRequest, "api_mode_disabled")
			return
		}
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if isLayerValidationError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventAgentDefUpdated, projectID, "", "", map[string]interface{}{
			"workflow_id": workflowID,
			"agent_id":    id,
		})
		s.wsHub.Broadcast(event)
	}

	resp := map[string]any{"status": "updated"}
	if req.Tools != nil {
		if warnings := toolsCSVWarnings(*req.Tools, s.availableAgentTools(projectID)); len(warnings) > 0 {
			resp["warnings"] = warnings
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleDeleteAgentDef deletes an agent definition
func (s *Server) handleDeleteAgentDef(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	workflowID := r.PathValue("wid")
	id := r.PathValue("id")

	svc := service.NewAgentDefinitionService(s.pool, s.clock, service.NewCLIModelService(s.pool, s.clock), service.NewAPIModelService(s.pool, s.clock), repo.NewPythonScriptRepo(s.pool, s.clock))

	if err := svc.DeleteAgentDef(projectID, workflowID, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventAgentDefDeleted, projectID, "", "", map[string]interface{}{
			"workflow_id": workflowID,
			"agent_id":    id,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
