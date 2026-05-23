package api

import (
	"net/http"
	"strings"

	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// handleListAPIModels returns all API models
func (s *Server) handleListAPIModels(w http.ResponseWriter, r *http.Request) {
	svc := service.NewAPIModelService(s.pool, s.clock)

	models, err := svc.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, models)
}

// handleCreateAPIModel creates a new API model
func (s *Server) handleCreateAPIModel(w http.ResponseWriter, r *http.Request) {
	var req types.APIModelCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := service.NewAPIModelService(s.pool, s.clock)

	m, err := svc.Create(req)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if strings.Contains(err.Error(), "required") ||
			strings.Contains(err.Error(), "invalid provider") ||
			strings.Contains(err.Error(), "invalid reasoning_effort") ||
			strings.Contains(err.Error(), "only supported on Anthropic Opus 4.7") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent(ws.EventAPIModelCreated, "", "", "", map[string]interface{}{
			"model_id": m.ID,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusCreated, m)
}

// handleGetAPIModel returns a single API model
func (s *Server) handleGetAPIModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	svc := service.NewAPIModelService(s.pool, s.clock)

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

// handleUpdateAPIModel updates an API model
func (s *Server) handleUpdateAPIModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req types.APIModelUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := service.NewAPIModelService(s.pool, s.clock)

	updated, err := svc.Update(id, req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "invalid reasoning_effort") ||
			strings.Contains(err.Error(), "only supported on Anthropic Opus 4.7") ||
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
		event := ws.NewEvent(ws.EventAPIModelUpdated, "", "", "", map[string]interface{}{
			"model_id": id,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteAPIModel deletes an API model
func (s *Server) handleDeleteAPIModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	svc := service.NewAPIModelService(s.pool, s.clock)

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
		event := ws.NewEvent(ws.EventAPIModelDeleted, "", "", "", map[string]interface{}{
			"model_id": id,
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
