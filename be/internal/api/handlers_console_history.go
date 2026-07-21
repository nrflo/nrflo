package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"be/internal/model"
	"be/internal/service"
	"be/internal/types"
)

// handleGetConsoleHistory returns the resolved project's recent console_chat
// 'user_input' message contents (oldest→newest), used to seed the native
// console TUI's Up/Down recall history from a project-scoped aggregate.
// GET /api/v1/console/history?limit=100
//
// Route is `protected`, not `projectAdmin` — auth mirrors
// handleListConsoleSkills exactly: a console/console_chat bearer is pinned
// unconditionally to its own session's project (X-Project/?project= is
// ignored), admin users and matching/global service tokens use
// getProjectID.
func (s *Server) handleGetConsoleHistory(w http.ResponseWriter, r *http.Request) {
	var projectID string
	if sess, ok := requireConsoleSession(r); ok {
		projectID = sess.ProjectID
	} else if u := getUser(r); u != nil && u.Role == model.UserRoleAdmin {
		projectID = getProjectID(r)
	} else if sp := getServicePrincipal(r); sp != nil {
		if sp.Global || (getProjectID(r) != "" && strings.EqualFold(getProjectID(r), sp.ProjectID)) {
			projectID = getProjectID(r)
		} else {
			writeError(w, http.StatusForbidden, "project scope required")
			return
		}
	} else {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project required")
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	messages, err := s.consoleChat.ProjectHistory(projectID, limit)
	if err != nil {
		if errors.Is(err, service.ErrConsoleProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, types.ConsoleHistoryResponse{ProjectID: projectID, Messages: messages})
}
