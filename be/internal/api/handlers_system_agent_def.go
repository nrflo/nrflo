package api

import (
	"net/http"
	"strings"

	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

func (s *Server) systemAgentDefinitionService() *service.SystemAgentDefinitionService {
	return service.NewSystemAgentDefinitionService(s.pool, s.clock, service.NewModelService(s.pool, s.clock))
}

// handleListSystemAgentDefs returns system agent definitions.
// Single-row endpoints (Get/PATCH/DELETE) intentionally still resolve api-mode rows
// so existing IDs remain reachable regardless of server mode.
func (s *Server) handleListSystemAgentDefs(w http.ResponseWriter, r *http.Request) {
	svc := s.systemAgentDefinitionService()

	settingsSvc := service.NewGlobalSettingsService(s.pool, s.clock)
	apiModeVal, _ := settingsSvc.Get("api_mode_enabled")
	defs, err := svc.ListForAPI(apiModeVal == "true")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, defs)
}

// handleCreateSystemAgentDef creates a new system agent definition
func (s *Server) handleCreateSystemAgentDef(w http.ResponseWriter, r *http.Request) {
	var req types.SystemAgentDefCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	svc := s.systemAgentDefinitionService()

	def, err := svc.Create(&req)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventSystemAgentDefCreated, "", "", "", map[string]interface{}{
			"agent_id": def.ID,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusCreated, def)
}

// handleGetSystemAgentDef returns a single system agent definition
func (s *Server) handleGetSystemAgentDef(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	svc := s.systemAgentDefinitionService()

	def, err := svc.Get(id)
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

// handleUpdateSystemAgentDef updates a system agent definition
func (s *Server) handleUpdateSystemAgentDef(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req types.SystemAgentDefUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := s.systemAgentDefinitionService()

	if err := svc.Update(id, &req); err != nil {
		writeServiceError(w, err)
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventSystemAgentDefUpdated, "", "", "", map[string]interface{}{
			"agent_id": id,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleDeleteSystemAgentDef deletes a system agent definition
func (s *Server) handleDeleteSystemAgentDef(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	svc := s.systemAgentDefinitionService()

	if err := svc.Delete(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventSystemAgentDefDeleted, "", "", "", map[string]interface{}{
			"agent_id": id,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
