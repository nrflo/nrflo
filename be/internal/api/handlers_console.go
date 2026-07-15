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

	// Optional current-ticket hint (e.g. the caller's git branch). Validated
	// against the project; an unknown id is silently dropped, never an error —
	// the session simply has no current ticket (silent fallback).
	var body struct {
		TicketID string `json:"ticket_id"`
	}
	_ = readJSON(r, &body)
	ticketID := strings.TrimSpace(body.TicketID)
	if ticketID != "" {
		if _, gerr := s.ticketService().Get(projectID, ticketID); gerr != nil {
			ticketID = ""
		}
	}

	consoleSvc := service.NewConsoleService(s.pool, s.clock)
	sessionID, token, err := consoleSvc.CreateSession(projectID, ticketID)
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
		"ticket_id":  ticketID,
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

// consolePrincipal is the authenticated identity behind a console request,
// snapshotted from the request context. The WS session channel needs the same
// authorization predicate as the REST routes but evaluates it long after the
// upgrade request is gone, so the predicate takes this instead of *http.Request.
type consolePrincipal struct {
	user     *model.User
	service  *ServicePrincipal
	ownAgent *model.AgentSession
}

// consolePrincipalOf snapshots the request's authenticated identity.
func consolePrincipalOf(r *http.Request) consolePrincipal {
	return consolePrincipal{
		user:     getUser(r),
		service:  getServicePrincipal(r),
		ownAgent: getAgentSession(r),
	}
}

// authorizedForConsoleSession reports whether p may act on sess: an admin
// user, a service principal (global or project-matching), or the console
// session's own bearer.
func authorizedForConsoleSession(p consolePrincipal, sess *model.AgentSession) bool {
	if p.user != nil && p.user.Role == model.UserRoleAdmin {
		return true
	}
	if p.service != nil {
		if p.service.Global || strings.EqualFold(p.service.ProjectID, sess.ProjectID) {
			return true
		}
	}
	if p.ownAgent != nil && p.ownAgent.ID == sess.ID {
		return true
	}
	return false
}

// authorizedForConsoleClose reports whether the request may close sess.
func authorizedForConsoleClose(r *http.Request, sess *model.AgentSession) bool {
	return authorizedForConsoleSession(consolePrincipalOf(r), sess)
}
