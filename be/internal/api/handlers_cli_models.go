package api

import (
	"net/http"
	"strings"

	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// handleListCLIModels returns all CLI models
func (s *Server) handleListCLIModels(w http.ResponseWriter, r *http.Request) {
	svc := service.NewCLIModelService(s.pool, s.clock)

	models, err := svc.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, models)
}

// handleCreateCLIModel creates a new CLI model
func (s *Server) handleCreateCLIModel(w http.ResponseWriter, r *http.Request) {
	var req types.CLIModelCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := service.NewCLIModelService(s.pool, s.clock)

	m, err := svc.Create(req)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if strings.Contains(err.Error(), "required") ||
			strings.Contains(err.Error(), "invalid cli_type") ||
			strings.Contains(err.Error(), "invalid reasoning_effort") ||
			strings.Contains(err.Error(), "only supported on Opus 4.7") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventCLIModelCreated, "", "", "", map[string]interface{}{
			"model_id": m.ID,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusCreated, m)
}

// handleGetCLIModel returns a single CLI model
func (s *Server) handleGetCLIModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	svc := service.NewCLIModelService(s.pool, s.clock)

	m, err := svc.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, m)
}

// handleUpdateCLIModel updates a CLI model
func (s *Server) handleUpdateCLIModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req types.CLIModelUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := service.NewCLIModelService(s.pool, s.clock)

	updated, err := svc.Update(id, req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "invalid reasoning_effort") ||
			strings.Contains(err.Error(), "only supported on Opus 4.7") ||
			strings.Contains(err.Error(), "only reasoning_effort can be updated on built-in models") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.Contains(err.Error(), "model is in use") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventCLIModelUpdated, "", "", "", map[string]interface{}{
			"model_id": id,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteCLIModel deletes a CLI model
func (s *Server) handleDeleteCLIModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	svc := service.NewCLIModelService(s.pool, s.clock)

	if err := svc.Delete(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "cannot delete system model") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventCLIModelDeleted, "", "", "", map[string]interface{}{
			"model_id": id,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
