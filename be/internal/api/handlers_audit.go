package api

import (
	"net/http"
	"strconv"

	"be/internal/model"
	"be/internal/repo"
)

// handleListAuditLog handles GET /api/v1/audit-log.
func (s *Server) handleListAuditLog(w http.ResponseWriter, r *http.Request) {
	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			page = p
		}
	}

	perPage := 50
	if v := r.URL.Query().Get("per_page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			if p < 1 {
				p = 1
			}
			if p > 200 {
				p = 200
			}
			perPage = p
		}
	}

	filter := model.AuditFilter{
		UserID: r.URL.Query().Get("user_id"),
		Action: r.URL.Query().Get("action"),
	}

	items, total, err := repo.NewAuditRepo(s.pool, s.clock).List(filter, page, perPage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []*model.AuditEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":    items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}
