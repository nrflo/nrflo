package api

import (
	"errors"
	"net/http"

	"be/internal/service"
	"be/internal/types"
)

// handleListConsoleSkills returns the resolved project's discovered
// .claude/skills for the console chat "/" suggestion dropdown.
// GET /api/v1/console/skills
func (s *Server) handleListConsoleSkills(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project required")
		return
	}
	skills, err := s.consoleChat.ListSkills(projectID)
	if err != nil {
		if errors.Is(err, service.ErrConsoleProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, types.ConsoleSkillsResponse{ProjectID: projectID, Skills: skills})
}
