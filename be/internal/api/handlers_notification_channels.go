package api

import (
	"net/http"
	"strconv"
	"strings"

	"be/internal/model"
	"be/internal/notify"
	"be/internal/service"
	"be/internal/types"
)

func (s *Server) handleGetNotificationVariables(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"variables": notify.AvailableVariables(),
		"defaults": map[string]string{
			string(model.ChannelKindSlack):    notify.DefaultTemplate(model.ChannelKindSlack),
			string(model.ChannelKindTelegram): notify.DefaultTemplate(model.ChannelKindTelegram),
		},
	})
}

func (s *Server) handleListNotificationChannels(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}
	wid := r.PathValue("wid")
	wfSvc := service.NewWorkflowService(s.pool, s.clock)
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker, wfSvc)
	channels, err := svc.List(projectID, wid)
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
	wid := r.PathValue("wid")
	var req types.NotificationChannelCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	wfSvc := service.NewWorkflowService(s.pool, s.clock)
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker, wfSvc)
	ch, err := svc.Create(projectID, wid, &req)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "already exists") {
			status = http.StatusConflict
		} else if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
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
	wid := r.PathValue("wid")
	id := r.PathValue("id")
	wfSvc := service.NewWorkflowService(s.pool, s.clock)
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker, wfSvc)
	ch, err := svc.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !strings.EqualFold(ch.ProjectID, projectID) || !strings.EqualFold(ch.WorkflowID, wid) {
		writeError(w, http.StatusNotFound, "notification channel not found")
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
	wid := r.PathValue("wid")
	id := r.PathValue("id")
	wfSvc := service.NewWorkflowService(s.pool, s.clock)
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker, wfSvc)

	// Verify ownership before update
	existing, err := svc.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !strings.EqualFold(existing.ProjectID, projectID) || !strings.EqualFold(existing.WorkflowID, wid) {
		writeError(w, http.StatusNotFound, "notification channel not found")
		return
	}

	var req types.NotificationChannelUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
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
	wid := r.PathValue("wid")
	id := r.PathValue("id")
	wfSvc := service.NewWorkflowService(s.pool, s.clock)
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker, wfSvc)

	// Verify ownership before delete
	existing, err := svc.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !strings.EqualFold(existing.ProjectID, projectID) || !strings.EqualFold(existing.WorkflowID, wid) {
		writeError(w, http.StatusNotFound, "notification channel not found")
		return
	}

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
	wid := r.PathValue("wid")
	id := r.PathValue("id")
	wfSvc := service.NewWorkflowService(s.pool, s.clock)
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker, wfSvc)

	// Verify ownership before test send
	existing, err := svc.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !strings.EqualFold(existing.ProjectID, projectID) || !strings.EqualFold(existing.WorkflowID, wid) {
		writeError(w, http.StatusNotFound, "notification channel not found")
		return
	}

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
	wid := r.PathValue("wid")
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
	wfSvc := service.NewWorkflowService(s.pool, s.clock)
	svc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker, wfSvc)

	// Verify channel belongs to this workflow
	ch, err := svc.Get(channelID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !strings.EqualFold(ch.ProjectID, projectID) || !strings.EqualFold(ch.WorkflowID, wid) {
		writeError(w, http.StatusNotFound, "notification channel not found")
		return
	}

	deliveries, err := svc.ListDeliveries(channelID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, deliveries)
}
