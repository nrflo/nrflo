package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"be/internal/console"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
)

// handleCreateConsoleChat mints a kind='console_chat' session and starts its
// engine. POST /api/v1/console/chats
func (s *Server) handleCreateConsoleChat(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project required")
		return
	}

	var body struct {
		Engine           string `json:"engine"`
		Model            string `json:"model"`
		ReasoningEffort  string `json:"reasoning_effort"`
		SystemTemplateID string `json:"system_template_id"`
		Profile          string `json:"profile"`
		RefineryEnabled  bool   `json:"refinery_enabled"`
	}
	raw, _ := io.ReadAll(r.Body)
	r.Body.Close() //nolint:errcheck
	if strings.TrimSpace(string(raw)) != "" {
		if err := json.Unmarshal(raw, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if body.Engine == "" {
		writeError(w, http.StatusBadRequest, "engine required")
		return
	}

	sessionID, err := s.consoleChat.Create(body.Engine, body.Model, body.ReasoningEffort, projectID, body.SystemTemplateID, body.Profile, body.RefineryEnabled)
	if err != nil {
		if errors.Is(err, service.ErrConsoleProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		if errors.Is(err, service.ErrAPIModeDisabled) {
			writeError(w, http.StatusBadRequest, "api mode is disabled")
			return
		}
		if errors.Is(err, console.ErrUnknownProfile) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	appendAudit(s, r, "console_chat.created", "agent_session", sessionID, "{}")

	writeJSON(w, http.StatusCreated, map[string]string{
		"session_id": sessionID,
		"engine":     body.Engine,
		"model":      body.Model,
		"status":     string(model.AgentSessionUserInteractive),
	})
}

// loadConsoleChatSession resolves {sid} to a kind='console_chat' row and
// authorizes the request (admin user, matching/global service principal, or
// the session's own bearer — authorizedForConsoleClose, handlers_console.go).
// Writes the error response itself on failure.
func (s *Server) loadConsoleChatSession(w http.ResponseWriter, r *http.Request) (*model.AgentSession, bool) {
	sid := r.PathValue("sid")
	sess, err := repo.NewAgentSessionRepo(s.pool, s.clock).GetConsoleChat(sid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if sess == nil {
		writeError(w, http.StatusNotFound, "console chat session not found")
		return nil, false
	}
	if !authorizedForConsoleClose(r, sess) {
		writeError(w, http.StatusForbidden, "not authorized for this console chat session")
		return nil, false
	}
	return sess, true
}

// handleConsoleChatMessage submits one user turn.
// POST /api/v1/console/chats/{sid}/messages
func (s *Server) handleConsoleChatMessage(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadConsoleChatSession(w, r)
	if !ok {
		return
	}

	var body struct {
		Text string `json:"text"`
	}
	raw, _ := io.ReadAll(r.Body)
	r.Body.Close() //nolint:errcheck
	if err := json.Unmarshal(raw, &body); err != nil || strings.TrimSpace(body.Text) == "" {
		writeError(w, http.StatusBadRequest, "text required")
		return
	}

	if err := s.consoleChat.SendMessage(sess.ID, body.Text); err != nil {
		if errors.Is(err, spawner.ErrTurnActive) {
			writeError(w, http.StatusConflict, "a turn is already active")
			return
		}
		if errors.Is(err, console.ErrChatSessionNotFound) {
			writeError(w, http.StatusNotFound, "console chat session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	appendAudit(s, r, "console_chat.message", "agent_session", sess.ID, "{}")
	w.WriteHeader(http.StatusAccepted)
}

// handleConsoleChatApproval resolves a pending approval; the wire vocabulary
// (spawner.ApprovalApprove/ApprovalDeny) is mapped here, never by ChatService.
// POST /api/v1/console/chats/{sid}/approvals/{aid}
func (s *Server) handleConsoleChatApproval(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadConsoleChatSession(w, r)
	if !ok {
		return
	}
	aid := r.PathValue("aid")

	var body struct {
		Decision string `json:"decision"`
	}
	raw, _ := io.ReadAll(r.Body)
	r.Body.Close() //nolint:errcheck
	if err := json.Unmarshal(raw, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var decision spawner.ApprovalDecision
	switch body.Decision {
	case "allow":
		decision = spawner.ApprovalApprove
	case "allow_for_session":
		decision = spawner.ApprovalApproveForSession
	case "deny":
		decision = spawner.ApprovalDeny
	default:
		writeError(w, http.StatusBadRequest, "decision must be allow, allow_for_session, or deny")
		return
	}

	if err := s.consoleChat.ReplyApproval(sess.ID, aid, decision); err != nil {
		if errors.Is(err, console.ErrChatSessionNotFound) {
			writeError(w, http.StatusNotFound, "console chat session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	appendAudit(s, r, "console_chat.approval", "agent_session", sess.ID, "{}")
	w.WriteHeader(http.StatusNoContent)
}

// handleRevokeConsoleChatSessionApproval removes one tool from the chat's
// approve_for_session allowlist so its next use asks the human again.
// DELETE /api/v1/console/chats/{sid}/session-approvals/{tool}
func (s *Server) handleRevokeConsoleChatSessionApproval(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadConsoleChatSession(w, r)
	if !ok {
		return
	}
	tool := r.PathValue("tool")
	if tool == "" {
		writeError(w, http.StatusBadRequest, "tool required")
		return
	}

	if err := s.consoleChat.RevokeSessionApproval(sess.ID, tool); err != nil {
		if errors.Is(err, console.ErrChatSessionNotFound) {
			writeError(w, http.StatusNotFound, "console chat session not found")
			return
		}
		// The codex engine cannot revoke (its allowlist is the app-server's).
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	appendAudit(s, r, "console_chat.session_approval_revoked", "agent_session", sess.ID, "{}")
	w.WriteHeader(http.StatusNoContent)
}

// handleCloseConsoleChat closes a console-chat session, killing its bearer
// token via CloseConsoleChat's status filter.
// POST /api/v1/console/chats/{sid}/close
func (s *Server) handleCloseConsoleChat(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadConsoleChatSession(w, r)
	if !ok {
		return
	}

	if err := s.consoleChat.Close(sess.ID); err != nil {
		if errors.Is(err, console.ErrChatSessionNotFound) {
			writeError(w, http.StatusNotFound, "console chat session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	appendAudit(s, r, "console_chat.closed", "agent_session", sess.ID, "{}")
	w.WriteHeader(http.StatusNoContent)
}

// handleInterruptConsoleChat cancels the active turn but keeps the engine and
// conversation alive. POST /api/v1/console/chats/{sid}/interrupt
func (s *Server) handleInterruptConsoleChat(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadConsoleChatSession(w, r)
	if !ok {
		return
	}
	if err := s.consoleChat.Interrupt(r.Context(), sess.ID); err != nil {
		switch {
		case errors.Is(err, spawner.ErrNoActiveTurn):
			writeError(w, http.StatusConflict, "no active turn")
		case errors.Is(err, console.ErrChatSessionNotFound):
			writeError(w, http.StatusNotFound, "console chat session not found")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	appendAudit(s, r, "console_chat.interrupted", "agent_session", sess.ID, "{}")
	w.WriteHeader(http.StatusAccepted)
}
