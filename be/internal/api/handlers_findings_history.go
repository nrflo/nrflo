package api

import (
	"net/http"
	"strconv"
	"time"

	"be/internal/repo"
)

type historyItem struct {
	ID          string  `json:"id"`
	FindingID   *string `json:"finding_id,omitempty"`
	Scope       string  `json:"scope"`
	ScopeID     string  `json:"scope_id"`
	Key         string  `json:"key"`
	Operation   string  `json:"operation"`
	OldValue    *string `json:"old_value"`
	NewValue    *string `json:"new_value"`
	ActorID     string  `json:"actor_id"`
	ActorSource string  `json:"actor_source"`
	CreatedAt   time.Time `json:"created_at"`
}

// handleListFindingsHistory returns paginated findings_history rows for a scope.
// GET /api/v1/findings/history
func (s *Server) handleListFindingsHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	scope := q.Get("scope")
	if scope == "" {
		writeError(w, http.StatusBadRequest, "scope is required")
		return
	}
	if scope != "session" && scope != "workflow_instance" && scope != "project" {
		writeError(w, http.StatusBadRequest, "invalid scope")
		return
	}

	scopeID := q.Get("scope_id")
	if scopeID == "" {
		writeError(w, http.StatusBadRequest, "scope_id is required")
		return
	}

	key := q.Get("key")

	limit := 50
	if ls := q.Get("limit"); ls != "" {
		if v, err := strconv.Atoi(ls); err == nil {
			limit = v
		}
	}
	if limit > 200 {
		limit = 200
	}
	if limit < 0 {
		limit = 50
	}

	offset := 0
	if os := q.Get("offset"); os != "" {
		if v, err := strconv.Atoi(os); err == nil {
			if v < 0 {
				writeError(w, http.StatusBadRequest, "offset must not be negative")
				return
			}
			offset = v
		}
	}

	projectID, ok := s.resolveHistoryScopeProject(w, scope, scopeID)
	if !ok {
		return
	}

	if _, err := repo.NewProjectRepo(s.pool, s.clock).Get(projectID); err != nil {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	rows, err := repo.NewFindingRepo(s.pool, s.clock).ListHistory(scope, scopeID, key, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]historyItem, 0, len(rows))
	for _, h := range rows {
		item := historyItem{
			ID:          h.ID,
			Scope:       h.Scope,
			ScopeID:     h.ScopeID,
			Key:         h.Key,
			Operation:   h.Operation,
			ActorID:     h.ActorID,
			ActorSource: h.ActorSource,
			CreatedAt:   h.CreatedAt,
		}
		if h.FindingID.Valid {
			v := h.FindingID.String
			item.FindingID = &v
		}
		if h.OldValue.Valid {
			v := h.OldValue.String
			item.OldValue = &v
		}
		if h.NewValue.Valid {
			v := h.NewValue.String
			item.NewValue = &v
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"limit":  limit,
		"offset": offset,
	})
}

// resolveHistoryScopeProject resolves the project ID for a given scope/scopeID pair.
// Returns the project ID and true on success; writes a 403 and returns false on failure.
func (s *Server) resolveHistoryScopeProject(w http.ResponseWriter, scope, scopeID string) (string, bool) {
	switch scope {
	case "project":
		return scopeID, true
	case "workflow_instance":
		wi, err := repo.NewWorkflowInstanceRepo(s.pool, s.clock).Get(scopeID)
		if err != nil || wi == nil {
			writeError(w, http.StatusForbidden, "access denied")
			return "", false
		}
		return wi.ProjectID, true
	case "session":
		sess, err := repo.NewAgentSessionRepo(s.pool, s.clock).Get(scopeID)
		if err != nil || sess == nil {
			writeError(w, http.StatusForbidden, "access denied")
			return "", false
		}
		return sess.ProjectID, true
	}
	writeError(w, http.StatusForbidden, "access denied")
	return "", false
}
