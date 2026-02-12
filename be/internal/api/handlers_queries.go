package api

import (
	"net/http"

	"be/internal/repo"
)

// handleSearch performs FTS5 search
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ticketRepo, _, database, err := s.getRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	tickets, err := ticketRepo.SearchWithBlockedInfo(projectID, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if tickets == nil {
		tickets = []*repo.PendingTicket{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tickets": tickets,
		"query":   query,
	})
}
