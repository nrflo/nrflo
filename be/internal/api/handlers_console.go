package api

import (
	"errors"
	"net/http"
	"strings"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// handleCreateConsoleSession mints a console session for the project resolved
// from X-Project / ?project=. Returns the bearer token exactly once.
// POST /api/v1/console/sessions
func (s *Server) handleCreateConsoleSession(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project required")
		return
	}

	consoleSvc := service.NewConsoleService(s.pool, s.clock)
	sessionID, token, err := consoleSvc.CreateSession(projectID)
	if err != nil {
		if errors.Is(err, service.ErrConsoleProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	appendAudit(s, r, "console.session.created", "agent_session", sessionID, "{}")

	writeJSON(w, http.StatusCreated, map[string]string{
		"session_id": sessionID,
		"token":      token,
	})
}

// handleCloseConsoleSession closes a console session, killing its bearer
// token. Authorized for an admin user, a matching/global service principal,
// or the console session's own bearer.
// POST /api/v1/console/sessions/{sid}/close
func (s *Server) handleCloseConsoleSession(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")

	sessRepo := repo.NewAgentSessionRepo(s.pool, s.clock)
	sess, err := sessRepo.GetConsole(sid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sess == nil {
		writeError(w, http.StatusNotFound, "console session not found")
		return
	}
	if !authorizedForConsoleClose(r, sess) {
		writeError(w, http.StatusForbidden, "not authorized to close this console session")
		return
	}

	consoleSvc := service.NewConsoleService(s.pool, s.clock)
	if err := consoleSvc.CloseSession(sid); err != nil {
		if errors.Is(err, service.ErrConsoleSessionNotFound) {
			writeError(w, http.StatusNotFound, "console session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	appendAudit(s, r, "console.session.closed", "agent_session", sid, "{}")

	w.WriteHeader(http.StatusNoContent)
}

// authorizedForConsoleClose reports whether the request may close sess: an
// admin user, a service principal (global or project-matching), or the
// console session's own bearer.
func authorizedForConsoleClose(r *http.Request, sess *model.AgentSession) bool {
	if u := getUser(r); u != nil && u.Role == model.UserRoleAdmin {
		return true
	}
	if sp := getServicePrincipal(r); sp != nil {
		if sp.Global || strings.EqualFold(sp.ProjectID, sess.ProjectID) {
			return true
		}
	}
	if own := getAgentSession(r); own != nil && own.ID == sess.ID {
		return true
	}
	return false
}
