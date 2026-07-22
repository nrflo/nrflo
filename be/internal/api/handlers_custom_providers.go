package api

import (
	"net/http"
	"strings"

	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

func isCustomProviderValidationErr(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "required") ||
		strings.Contains(msg, "invalid custom provider name") ||
		strings.Contains(msg, "reserved for a built-in provider") ||
		strings.Contains(msg, "invalid base_url") ||
		strings.Contains(msg, "invalid api_wire")
}

func (s *Server) handleListCustomProviders(w http.ResponseWriter, _ *http.Request) {
	providers, err := service.NewCustomProviderService(s.pool, s.clock).List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, providers)
}

func (s *Server) handleGetCustomProvider(w http.ResponseWriter, r *http.Request) {
	p, err := service.NewCustomProviderService(s.pool, s.clock).Get(r.PathValue("name"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleCreateCustomProvider(w http.ResponseWriter, r *http.Request) {
	var req types.CustomProviderCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, err := service.NewCustomProviderService(s.pool, s.clock).Create(req)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "already exists"):
			writeError(w, http.StatusConflict, err.Error())
		case isCustomProviderValidationErr(err):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.broadcastCustomProviderEvent(ws.EventCustomProviderCreated, p.Name)
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleUpdateCustomProvider(w http.ResponseWriter, r *http.Request) {
	var req types.CustomProviderUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := r.PathValue("name")
	p, err := service.NewCustomProviderService(s.pool, s.clock).Update(name, req)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "not found"):
			writeError(w, http.StatusNotFound, err.Error())
		case strings.Contains(err.Error(), "is in use"):
			writeError(w, http.StatusConflict, err.Error())
		case isCustomProviderValidationErr(err):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.broadcastCustomProviderEvent(ws.EventCustomProviderUpdated, name)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteCustomProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := service.NewCustomProviderService(s.pool, s.clock).Delete(name)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "not found"):
			writeError(w, http.StatusNotFound, err.Error())
		case strings.Contains(err.Error(), "is in use"):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.broadcastCustomProviderEvent(ws.EventCustomProviderDeleted, name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) broadcastCustomProviderEvent(eventType, name string) {
	if s.wsHub == nil {
		return
	}
	s.wsHub.Broadcast(ws.NewEvent(eventType, "", "", "", map[string]interface{}{
		"custom_provider_name": name,
	}))
}
