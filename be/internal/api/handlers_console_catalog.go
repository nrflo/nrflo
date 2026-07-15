package api

import (
	"errors"
	"net/http"

	"be/internal/service"
)

// handleGetConsoleCatalog returns the server-owned engine/model catalogue and
// live resumable chats for the resolved project.
// GET /api/v1/console/catalog
func (s *Server) handleGetConsoleCatalog(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project required")
		return
	}
	catalog, err := s.consoleChat.Catalog(projectID)
	if err != nil {
		if errors.Is(err, service.ErrConsoleProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}
