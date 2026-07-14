package api

import (
	"net/http"
	"strconv"

	"be/internal/service"
)

// handleGetConsoleChatMessages serves paginated message history for a
// console-chat session — same response shape as handleGetSessionMessages
// (handlers_workflow.go) — after the {sid} kind guard + authz predicate.
// GET /api/v1/console/chats/{sid}/messages
func (s *Server) handleGetConsoleChatMessages(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadConsoleChatSession(w, r)
	if !ok {
		return
	}

	limit := 0
	offset := 0
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	category := r.URL.Query().Get("category")

	agentSvc := service.NewAgentService(s.pool, s.clock)
	messages, total, err := agentSvc.GetSessionMessages(sess.ID, limit, offset, category)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_id": sess.ID,
		"messages":   messages,
		"total":      total,
	})
}
