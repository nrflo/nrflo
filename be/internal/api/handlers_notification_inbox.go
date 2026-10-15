package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"be/internal/model"
	"be/internal/service"
)

func (s *Server) registerNotificationRoutes(protected func(string, http.HandlerFunc)) {
	// Notification variables (global, no project scope)
	protected("GET /api/v1/notification-channels/variables", s.handleGetNotificationVariables)

	// Notification channels (workflow-scoped)
	protected("GET /api/v1/workflows/{wid}/notification-channels", s.handleListNotificationChannels)
	protected("POST /api/v1/workflows/{wid}/notification-channels", s.handleCreateNotificationChannel)
	protected("GET /api/v1/workflows/{wid}/notification-channels/{id}", s.handleGetNotificationChannel)
	protected("PATCH /api/v1/workflows/{wid}/notification-channels/{id}", s.handleUpdateNotificationChannel)
	protected("DELETE /api/v1/workflows/{wid}/notification-channels/{id}", s.handleDeleteNotificationChannel)
	protected("POST /api/v1/workflows/{wid}/notification-channels/{id}/test", s.handleTestNotificationChannel)
	protected("GET /api/v1/workflows/{wid}/notification-deliveries", s.handleListNotificationDeliveries)

	protected("GET /api/v1/notifications/{id}", s.handleGetInboxNotification)
	protected("GET /api/v1/notifications/global/{id}", s.handleGetInboxNotification)
	protected("GET /api/v1/notifications", s.handleNotificationInbox)
	protected("GET /api/v1/notifications/global", s.handleNotificationInbox)
	protected("POST /api/v1/notifications/read", s.handleNotificationInboxRead)
	protected("POST /api/v1/notifications/global/read", s.handleNotificationInboxRead)
}

func inboxProject(w http.ResponseWriter, r *http.Request) (*string, bool) {
	if r.URL.Path == "/api/v1/notifications/global" || strings.HasPrefix(r.URL.Path, "/api/v1/notifications/global/") {
		if principal, _ := r.Context().Value(servicePrincipalKey).(*ServicePrincipal); principal != nil && !principal.Global {
			writeError(w, http.StatusForbidden, "global inbox requires global access")
			return nil, false
		}
		if r.Context().Value(agentSessionKey) != nil {
			writeError(w, http.StatusForbidden, "global inbox is unavailable to agent sessions")
			return nil, false
		}
		return nil, true
	}
	project := getProjectID(r)
	if project == "" {
		writeError(w, http.StatusBadRequest, "project required")
		return nil, false
	}
	if principal, _ := r.Context().Value(servicePrincipalKey).(*ServicePrincipal); principal != nil && !principal.Global && !strings.EqualFold(principal.ProjectID, project) {
		writeError(w, http.StatusForbidden, "notification project mismatch")
		return nil, false
	}
	if session, _ := r.Context().Value(agentSessionKey).(*model.AgentSession); session != nil && !strings.EqualFold(session.ProjectID, project) {
		writeError(w, http.StatusForbidden, "notification project mismatch")
		return nil, false
	}
	return &project, true
}

func (s *Server) handleNotificationInbox(w http.ResponseWriter, r *http.Request) {
	project, ok := inboxProject(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	svc := service.NewNotificationInboxService(s.pool, s.clock, s.wsHub)
	result, err := svc.List(project, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleNotificationInboxRead(w http.ResponseWriter, r *http.Request) {
	project, ok := inboxProject(w, r)
	if !ok {
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewNotificationInboxService(s.pool, s.clock, s.wsHub)
	if err := svc.MarkRead(project, req.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleGetInboxNotification(w http.ResponseWriter, r *http.Request) {
	project, ok := inboxProject(w, r)
	if !ok {
		return
	}
	svc := service.NewNotificationInboxService(s.pool, s.clock, s.wsHub)
	result, err := svc.Get(project, r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
