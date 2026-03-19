package api

import (
	"net/http"
	"strings"

	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// handleListSystemAgentDefs returns all system agent definitions
func (s *Server) handleListSystemAgentDefs(w http.ResponseWriter, r *http.Request) {
	svc := service.NewSystemAgentDefinitionService(s.pool, s.clock)

	defs, err := svc.List()
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

	svc := service.NewSystemAgentDefinitionService(s.pool, s.clock)

	def, err := svc.Create(&req)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
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

	svc := service.NewSystemAgentDefinitionService(s.pool, s.clock)

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

	svc := service.NewSystemAgentDefinitionService(s.pool, s.clock)

	if err := svc.Update(id, &req); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
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

	svc := service.NewSystemAgentDefinitionService(s.pool, s.clock)

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
