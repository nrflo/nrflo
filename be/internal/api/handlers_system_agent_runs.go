package api

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"be/internal/model"
	"be/internal/repo"
)

// handleListSystemAgentRuns handles GET /api/v1/system-agent-runs
// (admin-only, cross-project by design): merges recent tier/system-agent
// sessions with recent refinery folds, newest-first.
func (s *Server) handleListSystemAgentRuns(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			if p < 1 {
				p = 1
			}
			if p > 200 {
				p = 200
			}
			limit = p
		}
	}

	var since time.Time
	if v := r.URL.Query().Get("since"); v != "" {
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid since: must be RFC3339")
			return
		}
		since = parsed
	}

	sessions, err := s.agentSessionRepo().ListSystemAgentRuns(limit, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	folds, err := repo.NewRefineryRunRepo(s.pool, s.clock).ListRecent(limit, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]*model.SystemAgentRun, 0, len(sessions)+len(folds))
	items = append(items, sessions...)
	for _, f := range folds {
		items = append(items, &model.SystemAgentRun{
			Kind:               "refinery_fold",
			SessionID:          f.SessionID,
			WorkflowInstanceID: f.WorkflowInstanceID,
			NodeID:             f.NodeID,
			ProjectID:          f.ProjectID,
			ResolvedProvider:   f.Provider,
			ModelID:            f.Model,
			PromptTokens:       f.PromptTokens,
			OutputTokens:       f.OutputTokens,
			Status:             f.Status,
			Error:              f.Error,
			FoldCount:          f.FoldCount,
			CreatedAt:          f.FoldedAt,
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if len(items) > limit {
		items = items[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"limit": limit,
	})
}
