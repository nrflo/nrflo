package api

import (
	"net/http"
	"strings"

	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

func isModelValidationErr(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "required") ||
		strings.Contains(msg, "invalid provider") ||
		strings.Contains(msg, "invalid reasoning_effort") ||
		strings.Contains(msg, "invalid supported_efforts") ||
		strings.Contains(msg, "is not supported by this model") ||
		strings.Contains(msg, "does not support effort selection") ||
		strings.Contains(msg, "cli_model or api_model") ||
		strings.Contains(msg, "fallback_models") ||
		strings.Contains(msg, "can be updated on built-in models")
}

func (s *Server) handleListModels(w http.ResponseWriter, _ *http.Request) {
	models, err := service.NewModelService(s.pool, s.clock).List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func (s *Server) handleCreateModel(w http.ResponseWriter, r *http.Request) {
	var req types.ModelCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	m, err := service.NewModelService(s.pool, s.clock).Create(req)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "already exists"):
			writeError(w, http.StatusConflict, err.Error())
		case isModelValidationErr(err):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.broadcastModelEvent(ws.EventModelCreated, m.ID)
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleGetModel(w http.ResponseWriter, r *http.Request) {
	m, err := service.NewModelService(s.pool, s.clock).Get(r.PathValue("id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	var req types.ModelUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id := r.PathValue("id")
	m, err := service.NewModelService(s.pool, s.clock).Update(id, req)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "not found"):
			writeError(w, http.StatusNotFound, err.Error())
		case strings.Contains(err.Error(), "model is in use"):
			writeError(w, http.StatusConflict, err.Error())
		case isModelValidationErr(err):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.broadcastModelEvent(ws.EventModelUpdated, id)
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := service.NewModelService(s.pool, s.clock).Delete(id)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "not found"):
			writeError(w, http.StatusNotFound, err.Error())
		case strings.Contains(err.Error(), "model is in use"):
			writeError(w, http.StatusConflict, err.Error())
		case strings.Contains(err.Error(), "cannot delete system model"):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.broadcastModelEvent(ws.EventModelDeleted, id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) broadcastModelEvent(eventType, id string) {
	if s.wsHub == nil {
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(eventType, "", "", "", map[string]interface{}{
		"model_id": id,
	}))
}
