package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"be/internal/console"
)

// handleSwitchConsoleChatModel opens a sibling t0-decider chat under a
// different engine/model/effort, seeded with {sid}'s refinery digest, and
// leaves {sid}'s own engine live — a model change never mutates a
// t0-decider chat's running engine mid-conversation.
// POST /api/v1/console/chats/{sid}/switch-model
func (s *Server) handleSwitchConsoleChatModel(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadConsoleChatSession(w, r)
	if !ok {
		return
	}

	var body struct {
		Engine          string `json:"engine"`
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoning_effort"`
	}
	raw, _ := io.ReadAll(r.Body)
	r.Body.Close() //nolint:errcheck
	if strings.TrimSpace(string(raw)) != "" {
		if err := json.Unmarshal(raw, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	siblingID, err := s.consoleChat.SwitchModel(sess.ID, body.Engine, body.Model, body.ReasoningEffort)
	if err != nil {
		writeSiblingError(w, err)
		return
	}

	appendAudit(s, r, "console_chat.model_switched", "agent_session", sess.ID, "{}")
	writeJSON(w, http.StatusCreated, map[string]string{"sibling_session_id": siblingID})
}

// handleOpenHandsSibling opens a t0-hands sibling chat seeded with {sid}'s
// refinery digest, and leaves {sid}'s own engine live.
// POST /api/v1/console/chats/{sid}/hands-sibling
func (s *Server) handleOpenHandsSibling(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadConsoleChatSession(w, r)
	if !ok {
		return
	}

	siblingID, err := s.consoleChat.OpenHandsSibling(sess.ID)
	if err != nil {
		writeSiblingError(w, err)
		return
	}

	appendAudit(s, r, "console_chat.hands_sibling_opened", "agent_session", sess.ID, "{}")
	writeJSON(w, http.StatusCreated, map[string]string{"sibling_session_id": siblingID})
}

// writeSiblingError maps the sibling-flow error vocabulary: an unknown live
// session is 404, a non-t0-decider origin (the sibling flows' profile gate)
// or unknown profile is 400, anything else is a 500.
func writeSiblingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, console.ErrChatSessionNotFound):
		writeError(w, http.StatusNotFound, "console chat session not found")
	case errors.Is(err, console.ErrUnknownProfile), errors.Is(err, console.ErrSiblingRequiresT0Decider):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
