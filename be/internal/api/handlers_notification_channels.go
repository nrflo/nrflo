package api

import (
	"net/http"
	"strconv"
	"strings"

	"be/internal/service"
	"be/internal/types"
)

func (s *Server) handleListNotificationChannels(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker)
	channels, err := svc.List(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, channels)
}

func (s *Server) handleCreateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	var req types.NotificationChannelCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker)
	ch, err := svc.Create(projectID, &req)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "already exists") {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ch)
}

func (s *Server) handleGetNotificationChannel(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker)
	ch, err := svc.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ch)
}

func (s *Server) handleUpdateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	var req types.NotificationChannelUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker)
	ch, err := svc.Update(id, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ch)
}

func (s *Server) handleDeleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker)
	if _, err := svc.Delete(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestNotificationChannel(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	id := r.PathValue("id")
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker)
	if err := svc.TestSend(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
}

func (s *Server) handleListNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	channelID := r.URL.Query().Get("channel_id")
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id query param required")
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker)
	deliveries, err := svc.ListDeliveries(channelID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, deliveries)
}
