package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"be/internal/console"
	"be/internal/spawner"
)

// handleConsoleChatTools serves GET /api/v1/console/chats/{sid}/tools: the
// chat's own invokable catalogue (its profile's allowlist, or the full
// console catalogue for a profile-less chat) — same shape as
// GET /api/v1/console/tools, scoped to this session. Authorized under the
// shared chat predicate (loadConsoleChatSession), not requireConsoleSession:
// the web console's cookie-admin caller must be able to list a chat's tools
// too, not just the chat's own bearer.
func (s *Server) handleConsoleChatTools(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadConsoleChatSession(w, r)
	if !ok {
		return
	}

	reg, err := console.BuildRegistry(s.consoleDeps(), catalogueForSession(sess))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	specs := console.Specs(reg)
	tools := make([]consoleToolSummary, 0, len(specs))
	for _, sp := range specs {
		tools = append(tools, consoleToolSummary{Name: sp.Name, Description: sp.Description, InputSchema: sp.InputSchema})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"tools": tools})
}

// handleConsoleChatInvoke serves POST /api/v1/console/chats/{sid}/invoke: a
// deterministic, server-side tool call against the chat's own catalogue —
// never a model turn (409 while the chat's own turn is running). Thin:
// decode, delegate to ChatService.InvokeTool, map errors, audit regardless of
// outcome, write JSON.
func (s *Server) handleConsoleChatInvoke(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadConsoleChatSession(w, r)
	if !ok {
		return
	}

	var body struct {
		Tool        string          `json:"tool"`
		Arguments   json.RawMessage `json:"arguments"`
		InformModel bool            `json:"inform_model"`
	}
	raw, _ := io.ReadAll(r.Body)
	r.Body.Close() //nolint:errcheck
	if strings.TrimSpace(string(raw)) != "" {
		if err := json.Unmarshal(raw, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if strings.TrimSpace(body.Tool) == "" {
		writeError(w, http.StatusBadRequest, "tool required")
		return
	}

	start := s.clock.Now()
	result, err := s.consoleChat.InvokeTool(r.Context(), sess.ID, body.Tool, body.Arguments, body.InformModel)
	dur := s.clock.Now().Sub(start)

	outcome := "ok"
	switch {
	case errors.Is(err, console.ErrToolNotFound):
		outcome = "not_found"
	case err != nil:
		outcome = "error"
	case !result.OK:
		outcome = "tool_error"
	}
	appendConsoleToolAudit(s, r, sess, sess.ProjectID, body.Tool, body.Arguments, dur, outcome)

	if err != nil {
		switch {
		case errors.Is(err, console.ErrChatSessionNotFound):
			writeError(w, http.StatusNotFound, "console chat session not found")
		case errors.Is(err, spawner.ErrTurnActive):
			writeError(w, http.StatusConflict, "a turn is already active")
		case errors.Is(err, console.ErrToolNotFound):
			writeError(w, http.StatusNotFound, "unknown tool: "+body.Tool)
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":          result.OK,
		"result":      result.Result,
		"duration_ms": result.DurationMs,
		"informed":    result.Informed,
	})
}
