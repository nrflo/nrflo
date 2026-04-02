package api

import (
	"math"
	"net/http"
	"strconv"

	"be/internal/model"
	"be/internal/service"
)

// handleListErrors returns paginated error logs for a project.
func (s *Server) handleListErrors(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required (use X-Project header or ?project= query param)")
		return
	}

	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			page = p
		}
	}

	perPage := 20
	if v := r.URL.Query().Get("per_page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			if p > 100 {
				p = 100
			}
			perPage = p
		}
	}

	errorType := r.URL.Query().Get("type")

	svc := service.NewErrorService(s.pool, s.clock, s.wsHub)
	errors, total, err := svc.ListErrors(projectID, errorType, page, perPage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if errors == nil {
		errors = []*model.ErrorLog{}
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"errors":      errors,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}
